package run

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pmmp/pocketmine-helm/cmd/plugin-downloader/client"
	"github.com/pmmp/pocketmine-helm/cmd/plugin-downloader/options"
	"github.com/pmmp/pocketmine-helm/cmd/plugin-downloader/watcher"
	"k8s.io/klog/v2"
)

type controller interface{
	Run(stopCh <-chan struct{})
}

func Run() error {
	opts := options.Setup()

	client, err := client.New(opts.KubeConfig, opts.MasterUrl)
	if err != nil {
		return fmt.Errorf("%w during initializing client set", err)
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	controllers := []controller{
		watcher.New(client, opts.MountPath),
	}

	client.Start(stopCh)

	for _, ctl := range controllers {
		go func(ctl controller) {
			ctl.Run(stopCh)
		}(ctl)
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGTERM, syscall.SIGINT)
	<-signalCh

	klog.Error("Shutting down gracefully...")

	return nil
}
