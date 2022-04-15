package client

import (
	"fmt"

	pocketmineclientset "github.com/pmmp/pocketmine-helm/pkg/client/clientset/versioned"
	pocketminev1alpha1typedclientset "github.com/pmmp/pocketmine-helm/pkg/client/clientset/versioned/typed/pocketmine/v1alpha1"
	pocketmineexternalversions "github.com/pmmp/pocketmine-helm/pkg/client/informers/externalversions"
	pocketminev1alpha1informers "github.com/pmmp/pocketmine-helm/pkg/client/informers/externalversions/pocketmine/v1alpha1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	k8sInformerFactory        informers.SharedInformerFactory
	pocketmineInformerFactory pocketmineexternalversions.SharedInformerFactory

	Plugins         func(namespace string) pocketminev1alpha1typedclientset.PluginInterface
	PluginsInformer pocketminev1alpha1informers.PluginInformer
}

func New(kubeConfig, masterUrl string) (*Client, error) {
	var restConfig *rest.Config
	if kubeConfig == "" && masterUrl == "" {
		var err error
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("%w during creation of in-cluster REST config", err)
		}
	} else {
		var err error
		restConfig, err = clientcmd.BuildConfigFromFlags(masterUrl, kubeConfig)
		if err != nil {
			return nil, fmt.Errorf("%w during loading kubeconfig", err)
		}
	}

	k8sClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("%w during initializing apiserver client", err)
	}
	k8sInformerFactory := informers.NewSharedInformerFactory(k8sClient, 0)

	pocketmineClient, err := pocketmineclientset.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("%w during initializing apiserver client", err)
	}
	pocketmineInformerFactory := pocketmineexternalversions.NewSharedInformerFactory(pocketmineClient, 0)

	pluginsInformer := pocketmineInformerFactory.Pmmp().V1alpha1().Plugins()

	client := &Client{
		k8sInformerFactory:        k8sInformerFactory,
		pocketmineInformerFactory: pocketmineInformerFactory,
		Plugins:                   pocketmineClient.PmmpV1alpha1().Plugins,
		PluginsInformer:           pluginsInformer,
	}

	return client, nil
}

func (client *Client) Start(stopCh <-chan struct{}) {
	client.k8sInformerFactory.Start(stopCh)
	client.pocketmineInformerFactory.Start(stopCh)
}
