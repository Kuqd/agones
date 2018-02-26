// Copyright 2018 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gameserversets

import (
	"agones.dev/agones/pkg/apis/stable"
	"agones.dev/agones/pkg/apis/stable/v1alpha1"
	"agones.dev/agones/pkg/client/clientset/versioned"
	getterv1alpha1 "agones.dev/agones/pkg/client/clientset/versioned/typed/stable/v1alpha1"
	"agones.dev/agones/pkg/client/informers/externalversions"
	listerv1alpha1 "agones.dev/agones/pkg/client/listers/stable/v1alpha1"
	"agones.dev/agones/pkg/util/crd"
	"agones.dev/agones/pkg/util/runtime"
	"agones.dev/agones/pkg/util/webhooks"
	"agones.dev/agones/pkg/util/workerqueue"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	extclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"fmt"
)

var (
	// ErrNoGameServerSetOwner is returned when a GameServerSet can't be found as an owner
	// for a GameServer
	ErrNoGameServerSetOwner = errors.New("No GameServerSet owner for this GameServer")
)

// Controller is a the GameServerSet controller
type Controller struct {
	logger              *logrus.Entry
	crdGetter           v1beta1.CustomResourceDefinitionInterface
	gameServerGetter    getterv1alpha1.GameServersGetter
	gameServerLister    listerv1alpha1.GameServerLister
	gameServerSynced    cache.InformerSynced
	gameServerSetGetter getterv1alpha1.GameServerSetsGetter
	gameServerSetLister listerv1alpha1.GameServerSetLister
	gameServerSetSynced cache.InformerSynced
	workerqueue         *workerqueue.WorkerQueue
	recorder            record.EventRecorder
}

// NewController returns a new gameserver crd controller
func NewController(
	wh *webhooks.WebHook,
	kubeClient kubernetes.Interface,
	extClient extclientset.Interface,
	agonesClient versioned.Interface,
	agonesInformerFactory externalversions.SharedInformerFactory) *Controller {

	gameServers := agonesInformerFactory.Stable().V1alpha1().GameServers()
	gsInformer := gameServers.Informer()
	gameServerSets := agonesInformerFactory.Stable().V1alpha1().GameServerSets()
	gssInformer := gameServerSets.Informer()

	c := &Controller{
		crdGetter:           extClient.ApiextensionsV1beta1().CustomResourceDefinitions(),
		gameServerGetter:    agonesClient.StableV1alpha1(),
		gameServerLister:    gameServers.Lister(),
		gameServerSynced:    gsInformer.HasSynced,
		gameServerSetGetter: agonesClient.StableV1alpha1(),
		gameServerSetLister: gameServerSets.Lister(),
		gameServerSetSynced: gssInformer.HasSynced,
	}

	c.logger = runtime.NewLoggerWithType(c)
	c.workerqueue = workerqueue.NewWorkerQueue(c.syncGameServerSet, c.logger, stable.GroupName+".GameServerSetController")

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(c.logger.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	c.recorder = eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "gameserverset-controller"})

	// TODO: make it so you can't update the GameServer template
	//wh.AddHandler("/mutate", stablev1alpha1.Kind("GameServer"), admv1beta1.Create, c.creationHandler)

	gssInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: c.workerqueue.Enqueue,
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldGss := oldObj.(*v1alpha1.GameServerSet)
			newGss := newObj.(*v1alpha1.GameServerSet)
			if oldGss.Spec.Replicas != newGss.Spec.Replicas {
				c.workerqueue.Enqueue(newGss)
			}
		},
	})

	gsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: c.gameServerEventHandler,
		UpdateFunc: func(oldObj, newObj interface{}) {
			gs := newObj.(*v1alpha1.GameServer)
			// ignore if already being deleted
			if gs.Status.State == v1alpha1.Unhealthy && gs.ObjectMeta.DeletionTimestamp == nil {
				c.gameServerEventHandler(gs)
			}
		},
		DeleteFunc: c.gameServerEventHandler,
	})

	return c
}

// Run the GameServerSet controller. Will block until stop is closed.
// Runs threadiness number workers to process the rate limited queue
func (c *Controller) Run(threadiness int, stop <-chan struct{}) error {
	err := crd.WaitForEstablishedCRD(c.crdGetter, "gameserversets."+stable.GroupName, c.logger)
	if err != nil {
		return err
	}
	c.workerqueue.Run(threadiness, stop)
	return nil
}

func (c *Controller) gameServerEventHandler(obj interface{}) {
	gs := obj.(*v1alpha1.GameServer)
	ref := v1.GetControllerOf(gs)
	if ref == nil {
		return
	}
	gss, err := c.gameServerSetLister.GameServerSets(gs.ObjectMeta.Namespace).Get(ref.Name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			c.logger.WithField("ref", ref).Info("Owner GameServerSet no longer available for syncing")
		} else {
			runtime.HandleError(c.logger.WithField("gs", gs.ObjectMeta.Name).WithField("ref", ref),
				errors.Wrap(err, "error retrieving GameServer owner"))
		}

		return
	}
	c.workerqueue.Enqueue(gss)
}

// syncGameServer synchronises the GameServers for the Set,
// making sure there are aways as many GameServers as requested
func (c *Controller) syncGameServerSet(key string) error {
	c.logger.WithField("key", key).Info("Synchronising")

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		// don't return an error, as we don't want this retried
		runtime.HandleError(c.logger.WithField("key", key), errors.Wrapf(err, "invalid resource key"))
		return nil
	}

	gss, err := c.gameServerSetLister.GameServerSets(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			c.logger.WithField("key", key).Info("GameServerSet is no longer available for syncing")
			return nil
		}
		return errors.Wrapf(err, "error retrieving GameServerSet %s from namespace %s", name, namespace)
	}

	list, err := c.listGameServers(gss)
	if err != nil {
		return err
	}
	if err := c.syncUnhealthyGameServers(list); err != nil {
		return err
	}

	diff := gss.Spec.Replicas - int32(len(list))

	if diff > 0 {
		if err := c.syncMoreGameServers(gss, diff); err != nil {
			return err
		}
	} else if diff < 0 {
		if err := c.syncLessGameSevers(gss, list, -1*diff); err != nil {
			return err
		}
	}

	if err := c.syncGameServerSetState(gss, list); err != nil {
		return err
	}

	return nil
}

// listGameServers lists the GameServers for a given GameServerSet
func (c *Controller) listGameServers(gss *v1alpha1.GameServerSet) ([]*v1alpha1.GameServer, error) {
	list, err := c.gameServerLister.List(labels.SelectorFromSet(labels.Set{v1alpha1.GameServerSetGameServerLabel: gss.ObjectMeta.Name}))
	if err != nil {
		return list, errors.Wrapf(err, "error listing gameservers for gameserverset %s", gss.ObjectMeta.Name)
	}

	var result []*v1alpha1.GameServer
	for _, gs := range list {
		if v1.IsControlledBy(gs, gss) {
			result = append(result, gs)
		}
	}

	return result, nil
}

// syncUnhealthyGameServers deletes any unhealthy game servers (that are not already being deleted)
func (c *Controller) syncUnhealthyGameServers(list []*v1alpha1.GameServer) error {
	for _, gs := range list {
		if gs.Status.State == v1alpha1.Unhealthy && gs.ObjectMeta.DeletionTimestamp.IsZero() {
			err := c.gameServerGetter.GameServers(gs.ObjectMeta.Namespace).Delete(gs.ObjectMeta.Name, nil)
			if err != nil {
				return errors.Wrapf(err, "error deleting gameserver %s", gs.ObjectMeta.Name)
			}
		}
	}

	return nil
}

// syncMoreGameServers adds diff more GameServers to the set
func (c *Controller) syncMoreGameServers(gss *v1alpha1.GameServerSet, diff int32) error {
	c.logger.WithField("diff", diff).WithField("gameserverset", gss.ObjectMeta.Name).Info("Adding more gameservers")
	for i := int32(0); i < diff; i++ {
		gs := gss.GameServer()
		gs, err := c.gameServerGetter.GameServers(gs.Namespace).Create(gs)
		if err != nil {
			return errors.Wrapf(err, "error creating gameserver for gameserverset %s", gss.ObjectMeta.Name)
		}
		// TODO: write test
		c.recorder.Event(gs, corev1.EventTypeNormal, "SuccessfulCreate", fmt.Sprintf("Created GameServer: %s", gs.ObjectMeta.Name))
	}

	return nil
}

// syncLessGameSevers removes Ready GameServers from the set of GameServers
func (c *Controller) syncLessGameSevers(gss *v1alpha1.GameServerSet, list []*v1alpha1.GameServer, diff int32) error {
	c.logger.WithField("diff", diff).WithField("gameserverset", gss.ObjectMeta.Name).Info("Deleting gameservers")
	count := int32(0)
	for _, gs := range list {
		if diff <= count {
			return nil
		}

		if gs.Status.State != v1alpha1.Allocated {
			err := c.gameServerGetter.GameServers(gs.Namespace).Delete(gs.ObjectMeta.Name, nil)
			if err != nil {
				return errors.Wrapf(err, "error deleting gameserver for gameserverset %s", gss.ObjectMeta.Name)
			}
			// TODO: write test
			c.recorder.Event(gs, corev1.EventTypeNormal, "SuccessfulDelete", fmt.Sprintf("Deleted GameServer: %s", gs.ObjectMeta.Name))
			count++
		}
	}

	return nil
}

// syncGameServerSetState synchronises the GameServerSet State with active GameServer counts
// TODO: write test
func (c *Controller) syncGameServerSetState(gss *v1alpha1.GameServerSet, list []*v1alpha1.GameServer) error {
	rc := int32(0)
	for _, gs := range list {
		if gs.Status.State == v1alpha1.Ready {
			rc++
		}
	}

	status := v1alpha1.GameServerSetStatus{Replicas: int32(len(list)), ReadyReplicas: rc}
	if gss.Status != status {
		gssCopy := gss.DeepCopy()
		gssCopy.Status = status
		_, err := c.gameServerSetGetter.GameServerSets(gss.ObjectMeta.Namespace).Update(gssCopy)
		if err != nil {
			return errors.Wrapf(err, "error updating status on GameServerSet %s", gss.ObjectMeta.Name)
		}
	}
	return nil
}
