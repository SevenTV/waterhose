package configure

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func checkErr(err error) {
	if err != nil {
		zap.S().Fatalw("config",
			"error", err,
		)
	}
}

func New() *Config {
	initLogging("info")

	config := viper.New()

	// Default config
	b, _ := json.Marshal(Config{
		ConfigFile: "config.yaml",
	})
	tmp := viper.New()
	defaultConfig := bytes.NewReader(b)
	tmp.SetConfigType("json")
	checkErr(tmp.ReadConfig(defaultConfig))
	checkErr(config.MergeConfigMap(viper.AllSettings()))

	pflag.String("mode", "", "The running mode, `controller` or `edge`")
	pflag.String("config", "config.yaml", "Config file location")
	pflag.Bool("noheader", false, "Disable the startup header")

	pflag.Parse()
	checkErr(config.BindPFlags(pflag.CommandLine))

	// File
	config.SetConfigFile(config.GetString("config"))
	config.AddConfigPath(".")
	if err := config.ReadInConfig(); err == nil {
		checkErr(config.MergeInConfig())
	}

	BindEnvs(config, Config{})

	// Environment
	config.AutomaticEnv()
	config.SetEnvPrefix("TE")
	config.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	config.AllowEmptyEnv(true)

	// Print final config
	c := &Config{}
	checkErr(config.Unmarshal(&c))

	initLogging(c.Level)

	return c
}

func BindEnvs(config *viper.Viper, iface interface{}, parts ...string) {
	ifv := reflect.ValueOf(iface)
	ift := reflect.TypeOf(iface)
	for i := 0; i < ift.NumField(); i++ {
		v := ifv.Field(i)
		t := ift.Field(i)
		tv, ok := t.Tag.Lookup("mapstructure")
		if !ok {
			continue
		}
		switch v.Kind() {
		case reflect.Struct:
			BindEnvs(config, v.Interface(), append(parts, tv)...)
		default:
			_ = config.BindEnv(strings.Join(append(parts, tv), "."))
		}
	}
}

type Mode string

const (
	ModeMaster Mode = "master"
	ModeSlave  Mode = "slave"
)

type Config struct {
	Level      string `mapstructure:"level" json:"level"`
	ConfigFile string `mapstructure:"config" json:"config"`
	NoHeader   bool   `mapstructure:"noheader" json:"noheader"`
	Mode       Mode   `mapstructure:"mode" json:"mode"`

	K8S struct {
		NodeName string `mapstructure:"node_name" json:"node_name"`
	} `mapstructure:"k8s" json:"k8s"`

	Master struct {
		API struct {
			Bind     string `mapstructure:"bind" json:"bind"`
			HttpBind string `mapstructure:"http_bind" json:"http_bind"`
		} `mapstructure:"api" json:"api"`

		Irc struct {
			ChannelLimitPerSlave int    `mapstructure:"channel_limit_per_slave" json:"channel_limit_per_slave"`
			BotAccountID         string `mapstructure:"bot_account_id" json:"bot_account_id"`
		} `mapstructure:"irc" json:"irc"`

		Twitch struct {
			ClientID     string `mapstructure:"client_id" json:"client_id"`
			ClientSecret string `mapstructure:"client_secret" json:"client_secret"`
			RedirectURI  string `mapstructure:"redirect_uri" json:"redirect_uri"`
		} `mapstructure:"twitch" json:"twitch"`

		K8S struct {
			Enabled        bool   `mapstructure:"enabled" json:"enabled"`
			Namespace      string `mapstructure:"namespace" json:"namespace"`
			InCluster      bool   `mapstructure:"in_cluster" json:"in_cluster"`
			ConfigPath     string `mapstructure:"config_path" json:"config_path"`
			SatefulsetName string `mapstructure:"statefulset_name" json:"statefulset_name"`
		} `mapstructure:"k8s" json:"k8s"`

		Mongo struct {
			URI      string `mapstructure:"uri" json:"uri"`
			Database string `mapstructure:"database" json:"database"`
			Direct   bool   `mapstructure:"direct" bson:"direct"`
		} `mapstructure:"mongo" json:"mongo"`
	} `mapstructure:"master" json:"master"`

	Slave struct {
		API struct {
			GrpcDial string `mapstructure:"grpc_dial" json:"grpc_dial"`
		} `mapstructure:"api" json:"api"`
		IRC struct {
			ChannelLimitPerConn int `mapstructure:"channel_limit_per_conn" json:"channel_limit_per_conn"`
		} `mapstructure:"irc" json:"irc"`
	} `mapstructure:"slave" json:"slave"`

	Redis struct {
		MasterName string   `mapstructure:"master_name" json:"master_name"`
		Username   string   `mapstructure:"username" json:"username"`
		Password   string   `mapstructure:"password" json:"password"`
		Database   int      `mapstructure:"database" json:"database"`
		Addresses  []string `mapstructure:"addresses" json:"addresses"`
		Sentinel   bool     `mapstructure:"sentinel" json:"sentinel"`
	} `mapstructure:"redis" json:"redis"`
}

func (c Config) IsMaster() bool {
	return c.Mode == ModeMaster
}

func (c Config) IsSlave() bool {
	return c.Mode == ModeSlave
}
