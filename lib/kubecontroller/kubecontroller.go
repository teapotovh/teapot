package kubecontroller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

var (
	ErrNoGVK   = errors.New("no group-version-kind found for object")
	ErrTimeout = errors.New("timed out")
)

// getResourceName returns the resource name from a type (without instance).
func getResourceName[T runtime.Object](mapper meta.RESTMapper) (string, error) {
	objType := reflect.TypeFor[T]()
	if objType.Kind() == reflect.Ptr {
		objType = objType.Elem()
	}

	obj := reflect.New(objType).Interface().(runtime.Object)

	gvks, _, err := scheme.Scheme.ObjectKinds(obj)
	if err != nil {
		return "", fmt.Errorf("failed to get group-version-kind: %w", err)
	}

	if len(gvks) == 0 {
		return "", ErrNoGVK
	}

	gvk := gvks[0]

	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return "", fmt.Errorf("failed to get rest mapping: %w", err)
	}

	return mapping.Resource.Resource, nil
}

func getMapper(client *kubernetes.Clientset) (meta.RESTMapper, error) {
	dc := client.Discovery()

	groupResources, err := restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return nil, fmt.Errorf("error while getting all api group resources: %w", err)
	}

	return restmapper.NewDiscoveryRESTMapper(groupResources), nil
}

type Handler[Resource runtime.Object] = func(name string, resource Resource, exists bool) error

type ControllerConfig[Resource runtime.Object] struct {
	FieldSelctor fields.Selector
	Client       *kubernetes.Clientset
	Handler      Handler[Resource]
	Namespace    string
	NumRetries   int
}

// Controller implements a simple kubernetes controller that calls a callback
// function (in parallel) on each update to any resource being watched.
type Controller[Resource runtime.Object] struct {
	store      cache.Store
	queue      workqueue.TypedRateLimitingInterface[string]
	informer   cache.Controller
	logger     *slog.Logger
	handler    Handler[Resource]
	resource   string
	numRetries int
}

// NewController creates a new Controller.
func NewController[Resource runtime.Object](
	config ControllerConfig[Resource],
	logger *slog.Logger,
) (*Controller[Resource], error) {
	mapper, err := getMapper(config.Client)
	if err != nil {
		return nil, fmt.Errorf("error while getting the kubernetes resource mapper: %w", err)
	}

	resource, err := getResourceName[Resource](mapper)
	if err != nil {
		return nil, fmt.Errorf("error while getting the kubernetes resource name to watch for: %w", err)
	}

	var fs fields.Selector
	if config.FieldSelctor != nil {
		fs = config.FieldSelctor
	} else {
		fs = fields.Everything()
	}

	podListWatcher := cache.NewListWatchFromClient(
		config.Client.CoreV1().RESTClient(),
		resource,
		config.Namespace,
		fs,
	)
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]())

	// Bind the workqueue to a cache with the help of an informer. This way we make sure that
	// whenever the cache is updated, the resource key is added to the workqueue.
	// Note that when we finally process the item from the workqueue, we might see a newer version
	// of the resource than the version which was responsible for triggering the update.
	var res Resource

	store, informer := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: podListWatcher,
		ObjectType:    res,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				key, err := cache.MetaNamespaceKeyFunc(obj)
				if err == nil {
					queue.Add(key)
				}
			},
			UpdateFunc: func(old any, recent any) {
				key, err := cache.MetaNamespaceKeyFunc(recent)
				if err == nil {
					queue.Add(key)
				}
			},
			DeleteFunc: func(obj any) {
				// IndexerInformer uses a delta queue, therefore for deletes we have to use this
				// key function.
				key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
				if err == nil {
					queue.Add(key)
				}
			},
		},
	})

	numRetries := config.NumRetries
	if numRetries == 0 {
		// Default to 5 retries
		numRetries = 5
	}

	return &Controller[Resource]{
		logger: logger,

		resource: resource,
		informer: informer,
		store:    store,
		queue:    queue,

		numRetries: numRetries,
		handler:    config.Handler,
	}, nil
}

func (c *Controller[Resource]) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer c.queue.Done(key)

	// Invoke the method containing the business logic
	err := c.callHandler(key)
	// Handle the error if something went wrong during the execution of the business logic
	c.handleErr(err, key)

	return true
}

func (c *Controller[Resource]) callHandler(key string) error {
	obj, exists, err := c.store.GetByKey(key)
	if err != nil {
		c.logger.Warn("error while fetching kubernetes resource after update", "err", err)
		return err
	}

	var resource Resource
	if exists {
		resource = obj.(Resource)
	}

	return c.handler(key, resource, exists)
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *Controller[Resource]) handleErr(err error, key string) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(key)
		return
	}

	// This controller retries c.numRetries times if something goes wrong. After that, it stops trying.
	nrq := c.queue.NumRequeues(key)
	if nrq < c.numRetries {
		c.logger.Warn("error while handling kubernetes resource update", "retries", nrq, "err", err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)

		return
	}

	c.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	utilruntime.HandleError(err)
	c.logger.Info(
		"stopped retrying for update to resource",
		"retries",
		c.numRetries,
		"resource",
		c.resource,
		"err",
		err,
	)
}

// Run begins watching and syncing.
func (c *Controller[Resource]) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrashWithContext(ctx)

	// Let the workers stop when we are done
	defer c.queue.ShutDown()

	c.logger.Debug("starting the kubernetes controller", "resource", c.resource)

	go c.informer.RunWithContext(ctx)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForNamedCacheSyncWithContext(ctx, c.informer.HasSynced) {
		return fmt.Errorf("error waiting for caches to sync: %w", ErrTimeout)
	}

	for range workers {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	<-ctx.Done()
	c.logger.Debug("stopping the kubernetes controller", "resource", c.resource)

	return nil
}

func (c *Controller[Resource]) runWorker(_ context.Context) {
	for c.processNextItem() {
	}
}
