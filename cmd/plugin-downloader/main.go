package main

import (
	"os"

	"github.com/pmmp/pocketmine-helm/cmd/plugin-downloader/run"
	"k8s.io/klog/v2"
)

func main() {
	if err := run.Run(); err != nil {
		klog.Error(err)
		os.Exit(1)
	}
}
