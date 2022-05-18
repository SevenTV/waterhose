package instance

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
)

type K8S interface {
	GetStatefulSet(ctx context.Context, name string) (*appsv1.StatefulSet, error)
	CreateStatefulSet(ctx context.Context, statefulSet appsv1.StatefulSet) (*appsv1.StatefulSet, error)
	UpdateStatefulSet(ctx context.Context, statefulSet *appsv1.StatefulSet) (*appsv1.StatefulSet, error)
	DeleteStatefulSet(ctx context.Context, name string) error
}
