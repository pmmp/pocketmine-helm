package watcher

import (
	"bytes"
	"context"
	"errors"
	goerrors "errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"sync"
	"time"

	"github.com/pmmp/pocketmine-helm/cmd/plugin-downloader/client"
	pocketminev1alpha1 "github.com/pmmp/pocketmine-helm/pkg/apis/pocketmine/v1alpha1"
	pocketminev1alpha1typedclientset "github.com/pmmp/pocketmine-helm/pkg/client/clientset/versioned/typed/pocketmine/v1alpha1"
	pocketminev1alpha1lister "github.com/pmmp/pocketmine-helm/pkg/client/listers/pocketmine/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

type Controller struct {
	httpClient      *http.Client
	mountPath       string
	pluginsClient   func(namespace string) pocketminev1alpha1typedclientset.PluginInterface
	pluginsInformer cache.SharedIndexInformer
	pluginsLister   pocketminev1alpha1lister.PluginLister
	handler         *pluginEventHandler
}

func New(client *client.Client, mountPath string) *Controller {
	client.PluginsInformer.Lister()

	handler := &pluginEventHandler{
		set:            make(map[resourceIdentifier]struct{}),
		updateNotifier: make(chan struct{}, 1),
	}
	client.PluginsInformer.Informer().AddEventHandler(handler)

	return &Controller{
		httpClient:      &http.Client{},
		mountPath:       mountPath,
		pluginsClient:   client.Plugins,
		pluginsInformer: client.PluginsInformer.Informer(),
		pluginsLister:   client.PluginsInformer.Lister(),
		handler:         handler,
	}
}

func (c *Controller) Run(stopCh <-chan struct{}) {
	ctx := context.TODO()

	errWaitingForSync := errors.New("waiting for sync")
	retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return err == errWaitingForSync
	}, func() error {
		if !c.pluginsInformer.HasSynced() {
			klog.Infof("Waiting for initial sync")
			return errWaitingForSync
		}

		return nil
	})

	c.reconcileInitial(ctx)

	for {
		select {
		case <-c.handler.updateNotifier:
			set := c.handler.clear()

			ids := make([]resourceIdentifier, 0, len(set))
			for id := range set {
				ids = append(ids, id)
			}

			c.reconcileAll(ctx, ids)
		case <-stopCh:
			return
		}
	}
}

func (c *Controller) reconcileInitial(ctx context.Context) {
	plugins, err := c.pluginsLister.List(labels.Everything())
	if err != nil {
		klog.Fatalf("failed to list plugins: %v", err)
	}

	ids := make([]resourceIdentifier, 0, len(plugins))
	for _, plugin := range plugins {
		ids = append(ids, newResourceIdentifier(plugin))
	}

	c.reconcileAll(ctx, ids)
}

func (c *Controller) reconcileAll(ctx context.Context, ids []resourceIdentifier) {
	klog.Infof("Reconciling %d plugins", len(ids))

	reconcileCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	wg := sync.WaitGroup{}
	wg.Add(len(ids))

	for _, id := range ids {
		go c.reconcile(reconcileCtx, id, wg.Done)
	}

	wg.Wait()

	klog.Infof("Reconciliation complete")
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if goerrors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func crc32File(id string, path string) (uint32, error) {
	hash := crc32.NewIEEE()

	_, _ = hash.Write([]byte(id)) // infallible

	handle, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer handle.Close()

	buf := make([]byte, 4096)
	for {
		n, err := handle.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}

		_, _ = hash.Write(buf[:n]) // infallible
	}

	return hash.Sum32(), nil
}

func downloadPlugin(ctx context.Context, client *http.Client, source *pocketminev1alpha1.PluginSource, path string) error {
	if len(source.Data) > 0 {
		return ioutil.WriteFile(path, source.Data, os.FileMode(0o644))
	}

	if source.Http != nil {
		url := source.Http.Url

		timeoutCtx, cancelFunc := context.WithTimeout(ctx, time.Duration(source.Http.TimeoutSeconds)*time.Second)
		defer cancelFunc()

		req, err := http.NewRequestWithContext(timeoutCtx, "GET", url, bytes.NewReader([]byte{}))
		if err != nil {
			return fmt.Errorf("%w during HTTP request creation", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("%w during HTTP request", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("HTTP request failed with status code %d", resp.StatusCode)
		}

		writer, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("%w during opening file %s for write", err, path)
		}
		defer writer.Close()

		_, err = io.Copy(writer, resp.Body)
		if err != nil {
			return fmt.Errorf("%w during writing file %s", err, path)
		}

		return nil
	}

	return fmt.Errorf("invalid plugin source")
}

func (c *Controller) reconcile(ctx context.Context, id resourceIdentifier, wgDone func()) {
	defer wgDone()

	path := c.getPath(id)

	plugin, err := c.pluginsLister.Plugins(id.namespace).Get(id.name)

	if err != nil {
		// plugin should be deleted
		if !k8serrors.IsNotFound(err) {
			klog.Errorf("Error fetching plugin object %s/%s from lister: %s", id.namespace, id.name, path)
			return
		}

		if exists, err := fileExists(path); err != nil || exists {
			unlinkErr := os.Remove(path)
			if unlinkErr != nil && !goerrors.Is(unlinkErr, fs.ErrNotExist) {
				klog.Errorf("Error unlinking plugin from %s: %s", path, unlinkErr)
			}
		}
	} else {
		// check if plugin needs to be re-downloaded
		exists, err := fileExists(path)
		if err != nil {
			klog.Errorf("Cannot stat file %s: %s", path, err)
			exists = false // assume does not exist
		}

		hashBase := ""
		if plugin.Spec.Source.Http != nil {
			hashBase = plugin.Spec.Source.Http.Url
		}

		if exists && plugin.Status.ExpectedChecksum != nil {
			cksum, err := crc32File(hashBase, path)
			if err != nil {
				klog.Errorf("Cannot calculate checksum of file %s: %s", path, err)
				// continue to try to re-download it
			} else if cksum == *plugin.Status.ExpectedChecksum {
				// checksum matches, no need to re-download
				return
			}
		}

		if err := downloadPlugin(ctx, c.httpClient, &plugin.Spec.Source, path); err != nil {
			klog.Errorf("Error downloading plugin %s/%s: %s", id.namespace, id.name, err)
			return
		}

		cksum, err := crc32File(hashBase, path)
		if err != nil {
			klog.Errorf("Cannot calculate checksum of file %s: %s", path, err)
		} else if plugin.Status.ExpectedChecksum == nil || cksum != *plugin.Status.ExpectedChecksum {
			plugin.Status.ExpectedChecksum = &cksum

			if _, err := c.pluginsClient(id.namespace).Update(ctx, plugin, metav1.UpdateOptions{}); err != nil {
				if !k8serrors.IsConflict(err) {
					klog.Errorf("Cannot update plugin %s/%s: %s", id.namespace, id.name, err)
				}
				// Else, another process updated the same resource.
				// Do not retry on conflict, because the plugin spec might have been changed.
				// If the spec is unchanged, this means another process has updated the expected checksum,
				// which is probably the same as the current one anyway.
			}
		}
	}
}

func (c *Controller) getPath(id resourceIdentifier) string {
	namespacePath := path.Join(c.mountPath, id.namespace)

	err := os.Mkdir(namespacePath, os.FileMode(0o755))
	if err != nil && !os.IsExist(err) {
		klog.Errorf("Error creating namespace directory %s: %s", namespacePath, err)
	}

	return path.Join(namespacePath, fmt.Sprintf("%s.phar", id.name))
}

type pluginEventHandler struct {
	set            map[resourceIdentifier]struct{}
	mutex          sync.Mutex
	updateNotifier chan struct{}
}

type resourceIdentifier struct {
	namespace string
	name      string
}

func newResourceIdentifier(meta metav1.Object) resourceIdentifier {
	return resourceIdentifier{namespace: meta.GetNamespace(), name: meta.GetName()}
}

func (h *pluginEventHandler) clear() map[resourceIdentifier]struct{} {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if len(h.set) == 0 {
		return nil
	}

	ids := h.set
	h.set = make(map[resourceIdentifier]struct{})
	return ids
}

func (h *pluginEventHandler) notify(obj interface{}) {
	plugin := obj.(*pocketminev1alpha1.Plugin)

	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.set[newResourceIdentifier(plugin)] = struct{}{}

	select {
	case h.updateNotifier <- struct{}{}:
	default:
	}
}

func (h *pluginEventHandler) OnAdd(obj interface{}) {
	h.notify(obj)
}

func (h *pluginEventHandler) OnUpdate(oldObj interface{}, newObj interface{}) {
	h.notify(newObj)
}

func (h *pluginEventHandler) OnDelete(obj interface{}) {
	h.notify(obj)
}
