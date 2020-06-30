package main

import (
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	foov1alpha1 "example.com/foo-controller/api/foo/v1alpha1"
	fooclientset "example.com/foo-controller/generated/clientset/versioned"
	fooscheme "example.com/foo-controller/generated/clientset/versioned/scheme"
	fooinformers "example.com/foo-controller/generated/informers/externalversions"
	foolisters "example.com/foo-controller/generated/listers/foo/v1alpha1"
)

type Controller struct {
	kubeclientset          kubernetes.Interface
	apiextensionsclientset apiextensionsclientset.Interface
	informer               cache.SharedIndexInformer
	fooclientset           fooclientset.Interface
	lister                 foolisters.FooLister
	recorder               record.EventRecorder
	workqueue              workqueue.RateLimitingInterface
}

func NewController() *Controller {
	kubeconfig := os.Getenv("KUBECONFIG")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.Fatalf(err.Error())
	}

	kubeClient := kubernetes.NewForConfigOrDie(config)
	apiextensionsClient := apiextensionsclientset.NewForConfigOrDie(config)
	fooClient := fooclientset.NewForConfigOrDie(config)

	informerFactory := fooinformers.NewSharedInformerFactory(fooClient, time.Minute*1)
	informer := informerFactory.Foo().V1alpha1().Foos()
	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(object interface{}) {
			klog.Infof("Added: %v", object)
		},
		UpdateFunc: func(oldObject, newObject interface{}) {
			klog.Infof("Updated: %v", newObject)
		},
		DeleteFunc: func(object interface{}) {
			klog.Infof("Deleted: %v", object)
		},
	})
	informerFactory.Start(wait.NeverStop)

	utilruntime.Must(foov1alpha1.AddToScheme(fooscheme.Scheme))
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(fooscheme.Scheme, corev1.EventSource{Component: "foo-controller"})

	workqueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	return &Controller{
		kubeclientset:          kubeClient,
		apiextensionsclientset: apiextensionsClient,
		fooclientset:           fooClient,
		informer:               informer.Informer(),
		lister:                 informer.Lister(),
		recorder:               recorder,
		workqueue:              workqueue,
	}
}

func (c *Controller) Run() {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	klog.Infoln("Waiting cache to be synced.")
	// Handle timeout for syncing.
	timeout := time.NewTimer(time.Second * 30)
	timeoutCh := make(chan struct{})
	go func() {
		<-timeout.C
		timeoutCh <- struct{}{}
	}()
	if ok := cache.WaitForCacheSync(timeoutCh, c.informer.HasSynced); !ok {
		klog.Fatalln("Timeout expired during waiting for caches to sync.")
	}

	klog.Infoln("Starting custom controller.")
	select {}
}

func main() {
	controller := NewController()
	controller.Run()
}