package configure

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"k8s.io/client-go/util/homedir"
)

func checkErr(err error) {
	if err != nil {
		logrus.WithError(err).Fatal("config")
	}
}

func New() *Config {
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

	pflag.String("config", "config.yaml", "Config file location")
	pflag.Bool("noheader", false, "Disable the startup header")

	if home := homedir.HomeDir(); home != "" {
		pflag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		pflag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	pflag.Parse()
	checkErr(config.BindPFlags(pflag.CommandLine))

	// File
	config.SetConfigFile(config.GetString("config"))
	config.AddConfigPath(".")
	err := config.ReadInConfig()
	if err != nil {
		logrus.Warning(err)
		logrus.Info("Using default config")
	} else {
		checkErr(config.MergeInConfig())
	}

	BindEnvs(config, Config{})

	// Environment
	config.AutomaticEnv()
	config.SetEnvPrefix("TCC")
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

type Config struct {
	Level      string `mapstructure:"level" json:"level"`
	ConfigFile string `mapstructure:"config" json:"config"`
	NoHeader   bool   `mapstructure:"noheader" json:"noheader"`

	API struct {
		Bind     string `mapstructure:"bind" json:"bind"`
		HttpBind string `mapstructure:"http_bind" json:"http_bind"`
	} `mapstructure:"api" json:"api"`

	Irc struct {
		ChannelLimit int `mapstructure:"channel_limit" json:"channel_limit"`
		Accounts     struct {
			MainAccountID string `mapstructure:"main_account_id" json:"main_account_id"`
		} `mapstructure:"accounts" json:"accounts"`
	} `mapstructure:"irc" json:"irc"`

	Redis struct {
		Username  string   `mapstructure:"username" json:"username"`
		Password  string   `mapstructure:"password" json:"password"`
		Database  int      `mapstructure:"database" json:"database"`
		Addresses []string `mapstructure:"addresses" json:"addresses"`
		Sentinel  bool     `mapstructure:"sentinel" json:"sentinel"`
	} `mapstructure:"redis" json:"redis"`

	Twitch struct {
		ClientID     string `mapstructure:"client_id" json:"client_id"`
		ClientSecret string `mapstructure:"client_secret" json:"client_secret"`
		RedirectURI  string `mapstructure:"redirect_uri" json:"redirect_uri"`
	} `mapstructure:"twitch" json:"twitch"`

	K8S struct {
		Namespace  string `mapstructure:"namespace" json:"namespace"`
		InCluster  bool   `mapstructure:"in_cluster" json:"in_cluster"`
		ConfigPath string `mapstructure:"config_path" json:"config_path"`
	} `mapstructure:"k8s" json:"k8s"`
}
