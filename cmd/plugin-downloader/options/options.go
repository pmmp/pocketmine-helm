package options

import (
	"os"

	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
)

type Options struct {
	KubeConfig string
	MasterUrl  string
	MountPath  string
}

func Setup() *Options {
	options := &Options{}

	pflag.StringVar(&options.KubeConfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	pflag.StringVar(&options.MasterUrl, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	pflag.StringVar(&options.MountPath, "mount-path", "/mnt/plugins", "The path to download plugins to.")
	pflag.BoolP("verbose", "v", false, "Enable verbose logging")

	pflag.Parse()

	stat, err := os.Stat(options.MountPath)
	if err != nil {
		klog.Errorf("Invalid --mount-path: %s", err)
		os.Exit(1)
	}
	if !stat.IsDir() {
		klog.Errorf("Invalid --mount-path: Not a directory")
		os.Exit(1)
	}

	return options
}
