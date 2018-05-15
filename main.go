package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/a8uhnf/controller-test/pkg/handlers"
	"github.com/a8uhnf/controller-test/pkg/utils"
	api_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const maxRetries = 5

// Controller object
type Controller struct {
	logger       *logrus.Entry
	clientset    kubernetes.Interface
	queue        workqueue.RateLimitingInterface
	informer     cache.SharedIndexInformer
	eventHandler handlers.Handler
}

// Event indicate the informerEvent
type Event struct {
	key          string
	eventType    string
	namespace    string
	resourceType string
}

func main() {
	createInformerQueue()
}

func createInformerQueue() {
	var eventHandler handlers.Handler
	eventHandler = new(handlers.Logger)
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	var kubeClient kubernetes.Interface
	_, err := rest.InClusterConfig()

	if err != nil {
		kubeClient = utils.GetClientOutOfCluster()
	} else {

	}
	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().Secrets(meta_v1.NamespaceAll).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().Secrets(meta_v1.NamespaceAll).Watch(options)
			},
		},
		&api_v1.Secret{},
		0, //Skip resync
		cache.Indexers{},
	)

	c := newResourceController(kubeClient, eventHandler, informer, "secret")
	stopCh := make(chan struct{})
	defer close(stopCh)

	go c.Run(stopCh)

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
	})

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	signal.Notify(sigterm, syscall.SIGINT)
	<-sigterm
}

// Run will start the controller.
// StopCh channel is used to send interrupt signal to stop it.
func (c *Controller) Run(stopCh <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.ShutDown()

	c.logger.Info("Starting kubewatch controller")

	go c.informer.Run(stopCh)

	// wait for the caches to synchronize before starting the worker
	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	c.logger.Info("Kubewatch controller synced and ready")

	// runWorker will loop until "something bad" happens.  The .Until will
	// then rekick the worker after one second
	wait.Until(c.runWorker, time.Second, stopCh)
}

// HasSynced is required for the cache.Controller interface.
func (c *Controller) HasSynced() bool {
	return c.informer.HasSynced()
}

func (c *Controller) runWorker() {
	// processNextWorkItem will automatically wait until there's work available
	for c.processNextItem() {
		// continue looping
	}
}

// processNextWorkItem deals with one key off the queue.  It returns false
// when it's time to quit.
func (c *Controller) processNextItem() bool {
	// pull the next work item from queue.  It should be a key we use to lookup
	// something in a cache
	key, quit := c.queue.Get()
	if quit {
		return false
	}

	// you always have to indicate to the queue that you've completed a piece of
	// work
	defer c.queue.Done(key)

	// do your work on the key.
	err := c.processItem(key.(Event).key)
	if err == nil {
		// No error, tell the queue to stop tracking history
		c.queue.Forget(key)
	} else if c.queue.NumRequeues(key) < maxRetries {
		c.logger.Errorf("Error processing %s (will retry): %v", key, err)
		// requeue the item to work on later
		c.queue.AddRateLimited(key)
	} else {
		// err != nil and too many retries
		c.logger.Errorf("Error processing %s (giving up): %v", key, err)
		c.queue.Forget(key)
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) processItem(key string) error {
	c.logger.Infof("Processing change to Pod %s", key)

	obj, exists, err := c.informer.GetIndexer().GetByKey(key)
	if err != nil {
		return fmt.Errorf("Error fetching object with key %s from store: %v", key, err)
	}

	if !exists {
		c.eventHandler.ObjectDeleted(obj)
		return nil
	}

	c.eventHandler.ObjectCreated(obj)
	return nil
}

func newResourceController(client kubernetes.Interface, eventHandler handlers.Handler, informer cache.SharedIndexInformer, resourceType string) *Controller {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	var newEvent Event
	var err error
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(obj)
			newEvent.eventType = "create"
			newEvent.resourceType = resourceType
			logrus.WithField("pkg", "kubewatch-"+resourceType).Infof("Processing add to %v: %s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(old)
			newEvent.eventType = "update"
			newEvent.resourceType = resourceType
			logrus.WithField("pkg", "kubewatch-"+resourceType).Infof("Processing update to %v: %s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
		DeleteFunc: func(obj interface{}) {
			newEvent.key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			newEvent.eventType = "delete"
			newEvent.resourceType = resourceType
			newEvent.namespace = utils.GetObjectMetaData(obj).Namespace
			logrus.WithField("pkg", "kubewatch-"+resourceType).Infof("Processing delete to %v: %s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
	})

	return &Controller{
		logger:       logrus.WithField("pkg", "kubewatch-"+resourceType),
		clientset:    client,
		informer:     informer,
		queue:        queue,
		eventHandler: eventHandler,
	}
}
