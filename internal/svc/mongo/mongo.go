package mongo

import (
	"github.com/SevenTV/Common/mongo"
	rMongo "go.mongodb.org/mongo-driver/mongo"
)

const (
	CollectionNameChannels mongo.CollectionName = "channels"
)

type WriteModel = mongo.WriteModel

var NewUpdateOneModel = rMongo.NewUpdateOneModel
