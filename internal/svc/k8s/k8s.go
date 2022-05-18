package k8s

import (
	"context"

	"github.com/seventv/waterhose/internal/global"
	"github.com/seventv/waterhose/internal/instance"
	"go.uber.org/zap"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typedAppsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type k8sApi struct {
	gCtx              global.Context
	client            *kubernetes.Clientset
	statefulSetClient typedAppsv1.StatefulSetInterface
}

func New(gCtx global.Context) instance.K8S {
	var (
		config *restclient.Config
		err    error
	)
	if gCtx.Config().Master.K8S.InCluster {
		config, err = restclient.InClusterConfig()
		if err != nil {
			zap.S().Fatalw("failed to get k8s config",
				"error", err,
			)
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", gCtx.Config().Master.K8S.ConfigPath)
		if err != nil {
			zap.S().Fatalw("failed to get k8s config",
				"error", err,
			)
		}
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		zap.S().Fatal("failed to connect to k8s api",
			"error", err,
		)
	}

	statefulSetClient := client.AppsV1().StatefulSets(gCtx.Config().Master.K8S.Namespace)

	return &k8sApi{
		gCtx:              gCtx,
		client:            client,
		statefulSetClient: statefulSetClient,
	}
}

func (k *k8sApi) GetStatefulSet(ctx context.Context, name string) (*appsv1.StatefulSet, error) {
	return k.statefulSetClient.Get(ctx, name, v1.GetOptions{})
}

func (k *k8sApi) CreateStatefulSet(ctx context.Context, statefulSet appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	return k.statefulSetClient.Create(ctx, &statefulSet, v1.CreateOptions{})
}

func (k *k8sApi) UpdateStatefulSet(ctx context.Context, statefulSet *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	return k.statefulSetClient.Update(ctx, statefulSet, v1.UpdateOptions{})
}

func (k *k8sApi) DeleteStatefulSet(ctx context.Context, name string) error {
	return k.statefulSetClient.Delete(ctx, name, v1.DeleteOptions{})
}
