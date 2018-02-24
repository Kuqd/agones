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
	"sort"
	"strconv"
	"testing"
	"time"

	"agones.dev/agones/pkg/apis/stable/v1alpha1"
	"agones.dev/agones/pkg/util/webhooks"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

func TestControllerWatchGameServers(t *testing.T) {
	gss := defaultFixture()

	c, m := newFakeController()

	received := make(chan string)
	defer close(received)

	m.extClient.AddReactor("get", "customresourcedefinitions", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, newEstablishedCRD(), nil
	})
	gssWatch := watch.NewFake()
	m.agonesClient.AddWatchReactor("gameserversets", k8stesting.DefaultWatchReactor(gssWatch, nil))
	gsWatch := watch.NewFake()
	m.agonesClient.AddWatchReactor("gameservers", k8stesting.DefaultWatchReactor(gsWatch, nil))

	c.workerqueue.SyncHandler = func(name string) error {
		received <- name
		return nil
	}

	stop, cancel := startInformers(m, c.gameServerSynced)
	defer cancel()

	go func() {
		err := c.Run(1, stop)
		assert.Nil(t, err)
	}()

	f := func() string {
		select {
		case result := <-received:
			return result
		case <-time.After(3 * time.Second):
			assert.FailNow(t, "timeout occurred")
		}
		return ""
	}

	expected, err := cache.MetaNamespaceKeyFunc(gss)
	assert.Nil(t, err)

	// gss add
	logrus.Info("adding gss")
	gssWatch.Add(gss.DeepCopy())
	assert.Nil(t, err)
	assert.Equal(t, expected, f())
	// gss update
	logrus.Info("modify gss")
	gssCopy := gss.DeepCopy()
	gssCopy.Spec.Replicas = 5
	gssWatch.Modify(gssCopy)
	assert.Equal(t, expected, f())

	gs := gss.GameServer()
	gs.ObjectMeta.Name = "test-gs"
	// gs add
	logrus.Info("add gs")
	gsWatch.Add(gs.DeepCopy())
	assert.Equal(t, expected, f())

	// gs update
	gsCopy := gs.DeepCopy()
	gsCopy.Status = v1alpha1.GameServerStatus{State: v1alpha1.Ready}

	logrus.Info("modify gs - noop")
	gsWatch.Modify(gsCopy.DeepCopy())
	select {
	case <-received:
		assert.Fail(t, "Should be no value")
	case <-time.After(time.Second):
	}

	gsCopy.Status.State = v1alpha1.Unhealthy
	logrus.Info("modify gs - unhealthy")
	gsWatch.Modify(gsCopy.DeepCopy())
	assert.Equal(t, expected, f())

	// gs delete
	logrus.Info("delete gs")
	gsWatch.Delete(gsCopy.DeepCopy())
	assert.Equal(t, expected, f())
}

func TestSyncGameServerSet(t *testing.T) {
	t.Run("adding and deleting unhealthy gameservers", func(t *testing.T) {
		gss := defaultFixture()
		list := createGameServers(gss, 5)

		// make some as unhealthy
		list[0].Status.State = v1alpha1.Unhealthy

		deleted := false
		count := 0

		c, m := newFakeController()
		m.agonesClient.AddReactor("list", "gameserversets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1alpha1.GameServerSetList{Items: []v1alpha1.GameServerSet{*gss}}, nil
		})
		m.agonesClient.AddReactor("list", "gameservers", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1alpha1.GameServerList{Items: list}, nil
		})

		m.agonesClient.AddReactor("delete", "gameservers", func(action k8stesting.Action) (bool, runtime.Object, error) {
			da := action.(k8stesting.DeleteAction)
			deleted = true
			assert.Equal(t, "test-0", da.GetName())
			return true, nil, nil
		})
		m.agonesClient.AddReactor("create", "gameservers", func(action k8stesting.Action) (bool, runtime.Object, error) {
			ca := action.(k8stesting.CreateAction)
			gs := ca.GetObject().(*v1alpha1.GameServer)

			assert.True(t, v1.IsControlledBy(gs, gss))
			count++
			return true, gs, nil
		})

		_, cancel := startInformers(m, c.gameServerSetSynced, c.gameServerSynced)
		defer cancel()

		c.syncGameServerSet(gss.ObjectMeta.Namespace + "/" + gss.ObjectMeta.Name)

		assert.Equal(t, 5, count)
		assert.True(t, deleted, "A game servers should have been deleted")
	})

	t.Run("removing gamservers", func(t *testing.T) {
		gss := defaultFixture()
		list := createGameServers(gss, 15)
		count := 0

		c, m := newFakeController()
		m.agonesClient.AddReactor("list", "gameserversets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1alpha1.GameServerSetList{Items: []v1alpha1.GameServerSet{*gss}}, nil
		})
		m.agonesClient.AddReactor("list", "gameservers", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &v1alpha1.GameServerList{Items: list}, nil
		})
		m.agonesClient.AddReactor("delete", "gameservers", func(action k8stesting.Action) (bool, runtime.Object, error) {
			count++
			return true, nil, nil
		})

		_, cancel := startInformers(m, c.gameServerSetSynced, c.gameServerSynced)
		defer cancel()

		c.syncGameServerSet(gss.ObjectMeta.Namespace + "/" + gss.ObjectMeta.Name)

		assert.Equal(t, 5, count)
	})
}

func TestControllerListGameServers(t *testing.T) {
	gss := defaultFixture()

	gs1 := gss.GameServer()
	gs1.ObjectMeta.Name = "test-1"
	gs2 := gss.GameServer()
	assert.True(t, v1.IsControlledBy(gs2, gss))

	gs2.ObjectMeta.Name = "test-2"
	gs3 := v1alpha1.GameServer{ObjectMeta: v1.ObjectMeta{Name: "not-included"}}
	gs4 := gss.GameServer()
	gs4.ObjectMeta.OwnerReferences = nil

	c, m := newFakeController()
	m.agonesClient.AddReactor("list", "gameservers", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &v1alpha1.GameServerList{Items: []v1alpha1.GameServer{*gs1, *gs2, gs3, *gs4}}, nil
	})

	_, cancel := startInformers(m)
	defer cancel()

	list, err := c.listGameServers(gss)
	assert.Nil(t, err)

	// sort of stable ordering
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].ObjectMeta.Name < list[j].ObjectMeta.Name
	})
	assert.Equal(t, []*v1alpha1.GameServer{gs1, gs2}, list)
}

func TestControllerSyncUnhealthyGameServers(t *testing.T) {
	gss := defaultFixture()

	gs1 := gss.GameServer()
	gs1.ObjectMeta.Name = "test-1"
	gs1.Status = v1alpha1.GameServerStatus{State: v1alpha1.Unhealthy}

	gs2 := gss.GameServer()
	gs2.ObjectMeta.Name = "test-2"
	gs2.Status = v1alpha1.GameServerStatus{State: v1alpha1.Ready}

	gs3 := gss.GameServer()
	gs3.ObjectMeta.Name = "test-3"
	now := v1.Now()
	gs3.ObjectMeta.DeletionTimestamp = &now
	gs3.Status = v1alpha1.GameServerStatus{State: v1alpha1.Ready}

	deleted := false

	c, m := newFakeController()
	m.agonesClient.AddReactor("delete", "gameservers", func(action k8stesting.Action) (bool, runtime.Object, error) {
		deleted = true
		da := action.(k8stesting.DeleteAction)
		assert.Equal(t, gs1.ObjectMeta.Name, da.GetName())

		return true, nil, nil
	})

	_, cancel := startInformers(m)
	defer cancel()

	err := c.syncUnhealthyGameServers([]*v1alpha1.GameServer{gs1, gs2, gs3})
	assert.Nil(t, err)

	assert.True(t, deleted, "Deletion should have occured")
}

func TestSyncMoreGameServers(t *testing.T) {
	gss := defaultFixture()

	c, m := newFakeController()
	count := 0
	expected := 10

	m.agonesClient.AddReactor("create", "gameservers", func(action k8stesting.Action) (bool, runtime.Object, error) {
		ca := action.(k8stesting.CreateAction)
		gs := ca.GetObject().(*v1alpha1.GameServer)

		assert.True(t, v1.IsControlledBy(gs, gss))
		count++

		return true, gs, nil
	})

	_, cancel := startInformers(m)
	defer cancel()

	err := c.syncMoreGameServers(gss, int32(expected))
	assert.Nil(t, err)

	assert.Equal(t, expected, count)
}

func TestSyncLessGameServers(t *testing.T) {
	gss := defaultFixture()

	c, m := newFakeController()
	count := 0
	expected := 5

	list := createGameServers(gss, 10)

	// make some as unhealthy
	list[0].Status.State = v1alpha1.Allocated
	list[3].Status.State = v1alpha1.Allocated

	// gate
	assert.Equal(t, v1alpha1.Allocated, list[0].Status.State)
	assert.Equal(t, v1alpha1.Allocated, list[3].Status.State)

	m.agonesClient.AddReactor("list", "gameservers", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &v1alpha1.GameServerList{Items: list}, nil
	})
	m.agonesClient.AddReactor("delete", "gameservers", func(action k8stesting.Action) (bool, runtime.Object, error) {
		da := action.(k8stesting.DeleteAction)

		found := false
		for _, gs := range list {
			if gs.ObjectMeta.Name == da.GetName() {
				found = true
				assert.NotEqual(t, gs.Status.State, v1alpha1.Allocated)
			}
		}
		assert.True(t, found)
		count++

		return true, nil, nil
	})

	_, cancel := startInformers(m)
	defer cancel()

	list2, err := c.listGameServers(gss)
	assert.Nil(t, err)
	assert.Len(t, list2, 10)

	err = c.syncLessGameSevers(gss, list2, int32(expected))
	assert.Nil(t, err)

	assert.Equal(t, expected, count)
}

// defaultFixture creates the default GameServerSet fixture
func defaultFixture() *v1alpha1.GameServerSet {
	gss := &v1alpha1.GameServerSet{
		ObjectMeta: v1.ObjectMeta{Namespace: "default", Name: "test", UID: "1234"},
		Spec: v1alpha1.GameServerSetSpec{
			Replicas: 10,
			Template: v1alpha1.GameServerTemplateSpec{},
		},
	}
	return gss
}

// createGameServers create an array of GameServers from the GameServerSet
func createGameServers(gss *v1alpha1.GameServerSet, size int) []v1alpha1.GameServer {
	var list []v1alpha1.GameServer
	for i := 0; i < size; i++ {
		gs := gss.GameServer()
		gs.Name = gs.GenerateName + strconv.Itoa(i)
		gs.Status = v1alpha1.GameServerStatus{State: v1alpha1.Ready}
		list = append(list, *gs)
	}
	return list
}

// newFakeController returns a controller, backed by the fake Clientset
func newFakeController() (*Controller, mocks) {
	m := newMocks()
	wh := webhooks.NewWebHook("", "")
	c := NewController(wh, m.kubeClient, m.extClient, m.agonesClient, m.agonesInformerFactory)
	c.recorder = m.fakeRecorder
	return c, m
}

func newEstablishedCRD() *v1beta1.CustomResourceDefinition {
	return &v1beta1.CustomResourceDefinition{
		Status: v1beta1.CustomResourceDefinitionStatus{
			Conditions: []v1beta1.CustomResourceDefinitionCondition{{
				Type:   v1beta1.Established,
				Status: v1beta1.ConditionTrue,
			}},
		},
	}
}
