package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SevenTV/Common/redis"
	jsoniter "github.com/json-iterator/go"
	"github.com/nicklaw5/helix"
	"github.com/seventv/twitch-chat-controller/src/global"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/sha3"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type OAuthResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int      `json:"expires_in"`
	Scope        []string `json:"scope"`
	TokenType    string   `json:"token_type"`
}

type HttpServer struct {
	gCtx global.Context
	*fasthttp.Server
}

func (h *HttpServer) Start(addr string) error {
	h.Handler = func(ctx *fasthttp.RequestCtx) {
		switch string(ctx.URI().Path()) {
		case "/auth/login":
			h.AuthLoginHandler(ctx)
		case "/auth/callback":
			h.AuthCallbackHandler(ctx)
		case "/auth/success":
			h.AuthSuccessHandler(ctx)
		default:
			ctx.SetStatusCode(404)
		}
	}

	return h.ListenAndServe(addr)
}

func (h *HttpServer) AuthSuccessHandler(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(200)
	ctx.SetBodyString("Success!")
}

func (h *HttpServer) AuthCallbackHandler(ctx *fasthttp.RequestCtx) {
	state := ctx.URI().QueryArgs().Peek("state")
	code := ctx.URI().QueryArgs().Peek("code")

	cookieState := ctx.Request.Header.Cookie("twitch-chat-csrf")
	hash := sha3.New512()
	hash.Write(cookieState)
	cookieStateHash := hex.EncodeToString(hash.Sum(nil))
	hash.Reset()
	hash.Write(state)
	stateHash := hex.EncodeToString(hash.Sum(nil))
	if cookieStateHash != stateHash {
		ctx.SetStatusCode(403)
		ctx.SetBodyString("bad csrf token")
		return
	}

	formBody := url.Values{}
	formBody.Set("client_id", h.gCtx.Config().Twitch.ClientID)
	formBody.Set("client_secret", h.gCtx.Config().Twitch.ClientSecret)
	formBody.Set("code", string(code))
	formBody.Set("grant_type", "authorization_code")
	formBody.Set("redirect_uri", h.gCtx.Config().Twitch.RedirectURI)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://id.twitch.tv/oauth2/token", strings.NewReader(formBody.Encode()))
	if err != nil {
		ctx.SetStatusCode(500)
		ctx.SetBodyString("internal server error")
		logrus.Error("error on callback: ", err)
		return
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(formBody.Encode())))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		ctx.SetStatusCode(500)
		ctx.SetBodyString("internal server error")
		logrus.Error("error on callback: ", err)
		return
	}

	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		ctx.SetStatusCode(500)
		ctx.SetBodyString("internal server error")
		logrus.Error("error on callback: ", err)
		return
	}
	tokenResp := helix.AccessCredentials{}
	if err := json.Unmarshal(data, &tokenResp); err != nil {
		ctx.SetStatusCode(500)
		ctx.SetBodyString("bad response from twitch")
		logrus.Errorf("error on callback: %e - %s", err, data)
		return
	}

	req, err = http.NewRequestWithContext(ctx, "GET", "https://api.twitch.tv/helix/users", nil)
	if err != nil {
		ctx.SetStatusCode(500)
		ctx.SetBodyString("internal server error")
		logrus.Error("error on callback: ", err)
		return
	}

	req.Header.Add("Authorization", "Bearer "+tokenResp.AccessToken)
	req.Header.Add("Client-Id", h.gCtx.Config().Twitch.ClientID)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		ctx.SetStatusCode(500)
		ctx.SetBodyString("internal server error")
		logrus.Error("error on callback: ", err)
		return
	}

	defer resp.Body.Close()
	data, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		ctx.SetStatusCode(500)
		ctx.SetBodyString("internal server error")
		logrus.Error("error on callback: ", err)
		return
	}

	userResp := helix.ManyUsers{}
	if err := json.Unmarshal(data, &userResp); err != nil {
		ctx.SetStatusCode(500)
		ctx.SetBodyString("bad response from twitch")
		logrus.Errorf("error on callback: %e - %s", err, data)
		return
	}

	if len(userResp.Users) != 1 {
		ctx.SetStatusCode(500)
		ctx.SetBodyString("bad response from twitch")
		logrus.Errorf("error on callback: %e - %s", err, data)
		return
	}

	user := userResp.Users[0]
	tokenData, _ := json.MarshalToString(tokenResp)
	if err := h.gCtx.Inst().Redis.Set(context.Background(), redis.Key("twitch-chat:login:"+user.ID), strconv.Itoa(int(time.Now().Unix()))+"+"+tokenData); err != nil {
		ctx.SetStatusCode(500)
		ctx.SetBodyString("failed to update redis")
		logrus.Errorf("error on callback: %e", err)
		return
	}

	h.gCtx.Inst().Events.Publish("twitch-chat:login:"+user.ID, nil)
}

func (h *HttpServer) AuthLoginHandler(ctx *fasthttp.RequestCtx) {
	v := url.Values{}
	v.Set("client_id", h.gCtx.Config().Twitch.ClientID)
	v.Set("redirect_uri", h.gCtx.Config().Twitch.RedirectURI)
	v.Set("response_type", "code")
	v.Set("scope", "channel:moderate chat:edit chat:read whispers:read whispers:edit")

	buf := make([]byte, 128)
	_, _ = rand.Read(buf)
	state := hex.EncodeToString(buf)
	v.Set("state", state)

	cookie := fasthttp.Cookie{}
	cookie.SetKey("twitch-chat-csrf")
	cookie.SetValue(state)
	cookie.SetHTTPOnly(true)
	cookie.SetSecure(true)

	ctx.Response.Header.SetCookie(&cookie)

	ctx.Redirect(fmt.Sprintf("https://id.twitch.tv/oauth2/authorize?%s", v.Encode()), 302)
}
