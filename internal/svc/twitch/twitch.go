package twitch

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SevenTV/Common/dataloader"
	"github.com/SevenTV/Common/redis"
	jsoniter "github.com/json-iterator/go"
	"github.com/nicklaw5/helix"
	"github.com/seventv/waterhose/internal/global"
	"github.com/seventv/waterhose/internal/instance"
	"go.uber.org/zap"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type twitchController struct {
	mtx        sync.Mutex
	gCtx       global.Context
	userLoader *dataloader.DataLoader[string, helix.User]
}

func New(gCtx global.Context) instance.Twitch {
	return &twitchController{
		gCtx: gCtx,
		userLoader: dataloader.New(dataloader.Config[string, helix.User]{
			Fetch: func(keys []string) ([]helix.User, []error) {
				users := make([]helix.User, len(keys))
				errs := make([]error, len(keys))

				client, err := helix.NewClient(&helix.Options{
					ClientID:     gCtx.Config().Master.Twitch.ClientID,
					ClientSecret: gCtx.Config().Master.Twitch.ClientSecret,
					RedirectURI:  gCtx.Config().Master.Twitch.RedirectURI,
					HTTPClient: &http.Client{
						Timeout: time.Second * 10,
					},
					RateLimitFunc: func(r *helix.Response) error {
						epoch, err := strconv.Atoi(r.Header.Get("Ratelimit-Limit"))
						if err != nil {
							return err
						}
						diff := time.Until(time.Unix(int64(epoch), 0))
						zap.S().Infow("hit twitch ratelimit",
							"delay", diff,
						)
						if diff > 0 {
							time.Sleep(diff)
						}

						return nil
					},
				})
				if err != nil {
					zap.S().Errorw("twitch error",
						"error", err,
					)
					for i := 0; i < len(errs); i++ {
						errs[i] = err
					}

					return users, errs
				}
				ctx, cancel := context.WithTimeout(gCtx, time.Second*5)
				tkn, err := gCtx.Inst().Redis.Get(ctx, redis.Key("twitch-app-token"))
				cancel()
				if err != nil {
					zap.S().Debug("fetching twitch app token")
					rTkn, err := client.RequestAppAccessToken(nil)
					if err != nil {
						zap.S().Errorw("twitch error",
							"error", err,
						)
						for i := 0; i < len(errs); i++ {
							errs[i] = err
						}

						return users, errs
					}

					tkn = rTkn.Data.AccessToken
					ctx, cancel := context.WithTimeout(gCtx, time.Second*5)
					err = gCtx.Inst().Redis.Set(ctx, redis.Key("twitch-app-token"), tkn)
					cancel()
					if err != nil {
						zap.S().Errorw("twitch error",
							"error", err,
						)
						for i := 0; i < len(errs); i++ {
							errs[i] = err
						}

						return users, errs
					}
				}

				client.SetAppAccessToken(tkn)

				resp, err := client.GetUsers(&helix.UsersParams{
					IDs: keys,
				})
				if err != nil {
					zap.S().Errorw("twitch error",
						"error", err,
					)
					for i := 0; i < len(errs); i++ {
						errs[i] = err
					}

					return users, errs
				}

				mp := map[string]helix.User{}
				for _, v := range resp.Data.Users {
					mp[v.ID] = v
				}

				for i, v := range keys {
					if usr, ok := mp[v]; ok {
						users[i] = usr
					} else {
						errs[i] = fmt.Errorf("not found")
					}
				}

				return users, errs

			},
			Wait:     100 * time.Millisecond,
			MaxBatch: 100,
		}),
	}
}

func (t *twitchController) GetOAuth(ctx context.Context, id string) (helix.AccessCredentials, error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	data, err := t.gCtx.Inst().Redis.Get(ctx, redis.Key("twitch-chat:login:"+id))
	if err != nil {
		return helix.AccessCredentials{}, err
	}

	splits := strings.SplitN(data, "+", 2)
	sec, err := strconv.Atoi(splits[0])
	if err != nil {
		return helix.AccessCredentials{}, err
	}

	createdAt := time.Unix(int64(sec), 0)

	creds := helix.AccessCredentials{}
	if err := json.UnmarshalFromString(splits[1], &creds); err != nil {
		return creds, err
	}

	if createdAt.Add(time.Second * time.Duration(float64(creds.ExpiresIn)*0.7)).Before(time.Now()) {
		formBody := url.Values{}
		formBody.Set("client_id", t.gCtx.Config().Master.Twitch.ClientID)
		formBody.Set("client_secret", t.gCtx.Config().Master.Twitch.ClientSecret)
		formBody.Set("grant_type", "refresh_token")
		formBody.Set("refresh_token", string(creds.RefreshToken))

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.twitch.tv/oauth2/token", strings.NewReader(formBody.Encode()))
		if err != nil {
			return helix.AccessCredentials{}, err
		}

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Content-Length", strconv.Itoa(len(formBody.Encode())))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return helix.AccessCredentials{}, err
		}

		defer resp.Body.Close()
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return helix.AccessCredentials{}, err
		}

		if err := json.Unmarshal(data, &creds); err != nil {
			return helix.AccessCredentials{}, err
		}

		tokenData, _ := json.MarshalToString(creds)

		if err := t.gCtx.Inst().Redis.Set(ctx, redis.Key("twitch-chat:login:"+id), strconv.Itoa(int(time.Now().Unix()))+"+"+tokenData); err != nil {
			return helix.AccessCredentials{}, err
		}
	}

	return creds, nil
}

func (t *twitchController) GetUser(id string) (helix.User, error) {
	return t.userLoader.Load(id)
}

func (t *twitchController) GetUsers(ids []string) ([]helix.User, []error) {
	return t.userLoader.LoadAll(ids)
}
