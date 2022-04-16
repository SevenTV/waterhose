package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/seventv/twitch-edge/src/configure"
	"github.com/seventv/twitch-edge/src/global"
	"github.com/seventv/twitch-edge/src/instance"
)

type mon struct {
	mode             configure.Mode
	twitchEdgeMaster instance.MonitoringTwitchEdgeMaster
	twitchEdgeSlave  instance.MonitoringTwitchEdgeSlave
}

func (m *mon) TwitchEdgeMaster() instance.MonitoringTwitchEdgeMaster {
	return m.twitchEdgeMaster
}

func (m *mon) TwitchEdgeSlave() instance.MonitoringTwitchEdgeSlave {
	return m.twitchEdgeSlave
}

func (m *mon) Register(r prometheus.Registerer) {
	if m.mode == configure.ModeMaster {
		r.MustRegister(
			m.twitchEdgeMaster.TotalChannels,
		)
	} else if m.mode == configure.ModeSlave {
		r.MustRegister(
			m.twitchEdgeSlave.TotalChannelsBannedIn,
			m.twitchEdgeSlave.TotalChannelsSuspended,
			m.twitchEdgeSlave.TotalChannelsConnectedTo,
		)
	}
}

func labelsFromKeyValue(kv []configure.KeyValue) prometheus.Labels {
	mp := prometheus.Labels{}

	for _, v := range kv {
		mp[v.Key] = v.Value
	}

	return mp
}

func NewPrometheus(gCtx global.Context) instance.Monitoring {
	return &mon{
		mode: gCtx.Config().Mode,
		twitchEdgeMaster: instance.MonitoringTwitchEdgeMaster{
			TotalChannels: prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:        "total_channels",
				ConstLabels: labelsFromKeyValue(gCtx.Config().Monitoring.Labels),
				Help:        "The total number of channels",
			}),
		},
		twitchEdgeSlave: instance.MonitoringTwitchEdgeSlave{
			TotalChannelsBannedIn: prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:        "total_channels_banned_in",
				ConstLabels: labelsFromKeyValue(gCtx.Config().Monitoring.Labels),
				Help:        "The total number of channels the bot is banned in",
			}),
			TotalChannelsSuspended: prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:        "total_channels_suspended",
				ConstLabels: labelsFromKeyValue(gCtx.Config().Monitoring.Labels),
				Help:        "The total number of channels suspended",
			}),
			TotalChannelsConnectedTo: prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:        "total_channels_connected_to",
				ConstLabels: labelsFromKeyValue(gCtx.Config().Monitoring.Labels),
				Help:        "The total number of channels connected to",
			}),
			TotalMessages: prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:        "total_messages",
				ConstLabels: labelsFromKeyValue(gCtx.Config().Monitoring.Labels),
				Help:        "The total number of messages",
			}),
		},
	}
}
