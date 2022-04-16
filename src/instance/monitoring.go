package instance

import "github.com/prometheus/client_golang/prometheus"

type Monitoring interface {
	Register(prometheus.Registerer)
	TwitchEdgeMaster() MonitoringTwitchEdgeMaster
	TwitchEdgeSlave() MonitoringTwitchEdgeSlave
}

type MonitoringTwitchEdgeMaster struct {
	TotalChannels prometheus.Histogram
}

type MonitoringTwitchEdgeSlave struct {
	TotalChannelsBannedIn    prometheus.Histogram
	TotalChannelsSuspended   prometheus.Histogram
	TotalChannelsConnectedTo prometheus.Histogram
	TotalMessages            prometheus.Histogram
}
