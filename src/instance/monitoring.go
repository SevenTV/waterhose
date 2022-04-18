package instance

import "github.com/prometheus/client_golang/prometheus"

type Monitoring interface {
	Register(prometheus.Registerer)
	WaterHoseMaster() MonitoringWaterHoseMaster
	WaterHoseSlave() MonitoringWaterHoseSlave
}

type MonitoringWaterHoseMaster struct {
	TotalChannels prometheus.Histogram
}

type MonitoringWaterHoseSlave struct {
	TotalChannelsBannedIn    prometheus.Histogram
	TotalChannelsSuspended   prometheus.Histogram
	TotalChannelsConnectedTo prometheus.Histogram
	TotalMessages            prometheus.Histogram
}
