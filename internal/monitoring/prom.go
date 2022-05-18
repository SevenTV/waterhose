package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/seventv/waterhose/internal/configure"
	"github.com/seventv/waterhose/internal/global"
	"github.com/seventv/waterhose/internal/instance"
)

type mon struct {
	mode            configure.Mode
	waterhoseMaster instance.MonitoringWaterHoseMaster
	waterhoseSlave  instance.MonitoringWaterHoseSlave
}

func (m *mon) WaterHoseMaster() instance.MonitoringWaterHoseMaster {
	return m.waterhoseMaster
}

func (m *mon) WaterHoseSlave() instance.MonitoringWaterHoseSlave {
	return m.waterhoseSlave
}

func (m *mon) Register(r prometheus.Registerer) {
	if m.mode == configure.ModeMaster {
		r.MustRegister(
			m.waterhoseMaster.TotalChannels,
		)
	} else if m.mode == configure.ModeSlave {
		r.MustRegister(
			m.waterhoseSlave.TotalChannelsBannedIn,
			m.waterhoseSlave.TotalChannelsSuspended,
			m.waterhoseSlave.TotalChannelsConnectedTo,
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
		waterhoseMaster: instance.MonitoringWaterHoseMaster{
			TotalChannels: prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:        "total_channels",
				ConstLabels: labelsFromKeyValue(gCtx.Config().Monitoring.Labels),
				Help:        "The total number of channels",
			}),
		},
		waterhoseSlave: instance.MonitoringWaterHoseSlave{
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
