package finalizercontroller

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/util/workqueue"
	fakeworkclient "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workapiv1 "open-cluster-management.io/api/work/v1"
	"open-cluster-management.io/work/pkg/spoke/controllers"
	"open-cluster-management.io/work/pkg/spoke/spoketesting"
)

func TestFinalize(t *testing.T) {
	cases := []struct {
		name                               string
		existingFinalizers                 []string
		existingResources                  []runtime.Object
		resourcesToRemove                  []workapiv1.AppliedManifestResourceMeta
		terminated                         bool
		validateAppliedManifestWorkActions func(t *testing.T, actions []clienttesting.Action)
		validateDynamicActions             func(t *testing.T, actions []clienttesting.Action)
		expectedQueueLen                   int
	}{
		{
			name:                               "skip when not delete",
			existingFinalizers:                 []string{controllers.ManifestWorkFinalizer},
			validateAppliedManifestWorkActions: noAction,
			validateDynamicActions:             noAction,
		},
		{
			name:                               "skip when finalizer gone",
			terminated:                         true,
			existingFinalizers:                 []string{"other-finalizer"},
			validateAppliedManifestWorkActions: noAction,
			validateDynamicActions:             noAction,
		},
		{
			name:               "delete resources and remove finalizer",
			terminated:         true,
			existingFinalizers: []string{"a", controllers.AppliedManifestWorkFinalizer, "b"},
			resourcesToRemove: []workapiv1.AppliedManifestResourceMeta{
				{Group: "g1", Version: "v1", Resource: "r1", Namespace: "", Name: "n1"},
				{Group: "g2", Version: "v2", Resource: "r2", Namespace: "ns2", Name: "n2"},
				{Group: "g3", Version: "v3", Resource: "r3", Namespace: "ns3", Name: "n3"},
				{Group: "g4", Version: "v4", Resource: "r4", Namespace: "", Name: "n4"},
			},
			validateAppliedManifestWorkActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 2 {
					t.Fatal(spew.Sdump(actions))
				}

				work := actions[0].(clienttesting.UpdateAction).GetObject().(*workapiv1.AppliedManifestWork)
				if len(work.Status.AppliedResources) != 0 {
					t.Fatal(spew.Sdump(actions[0]))
				}
				work = actions[1].(clienttesting.UpdateAction).GetObject().(*workapiv1.AppliedManifestWork)
				if !reflect.DeepEqual(work.Finalizers, []string{"a", "b"}) {
					t.Fatal(spew.Sdump(actions[1]))
				}
			},
			validateDynamicActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 4 {
					t.Fatal(spew.Sdump(actions))
				}

				action := actions[0].(clienttesting.DeleteAction)
				resource, namespace, name := action.GetResource(), action.GetNamespace(), action.GetName()
				if !reflect.DeepEqual(resource, schema.GroupVersionResource{Group: "g1", Version: "v1", Resource: "r1"}) || namespace != "" || name != "n1" {
					t.Fatal(spew.Sdump(actions))
				}
				action = actions[1].(clienttesting.DeleteAction)
				resource, namespace, name = action.GetResource(), action.GetNamespace(), action.GetName()
				if !reflect.DeepEqual(resource, schema.GroupVersionResource{Group: "g2", Version: "v2", Resource: "r2"}) || namespace != "ns2" || name != "n2" {
					t.Fatal(spew.Sdump(actions))
				}
				action = actions[2].(clienttesting.DeleteAction)
				resource, namespace, name = action.GetResource(), action.GetNamespace(), action.GetName()
				if !reflect.DeepEqual(resource, schema.GroupVersionResource{Group: "g3", Version: "v3", Resource: "r3"}) || namespace != "ns3" || name != "n3" {
					t.Fatal(spew.Sdump(actions))
				}
				action = actions[3].(clienttesting.DeleteAction)
				resource, namespace, name = action.GetResource(), action.GetNamespace(), action.GetName()
				if !reflect.DeepEqual(resource, schema.GroupVersionResource{Group: "g4", Version: "v4", Resource: "r4"}) || namespace != "" || name != "n4" {
					t.Fatal(spew.Sdump(actions))
				}
			},
		},
		{
			name:               "requeue work when deleting resources are still visiable",
			terminated:         true,
			existingFinalizers: []string{controllers.AppliedManifestWorkFinalizer},
			existingResources: []runtime.Object{
				spoketesting.NewUnstructuredSecret("ns1", "n1", true, "ns1-n1"),
				spoketesting.NewUnstructuredSecret("ns2", "n2", true, "ns2-n2"),
			},
			resourcesToRemove: []workapiv1.AppliedManifestResourceMeta{
				{Version: "v1", Resource: "secrets", Namespace: "ns1", Name: "n1", UID: "ns1-n1"},
				{Version: "v1", Resource: "secrets", Namespace: "ns2", Name: "n2", UID: "ns2-n2"},
			},
			validateAppliedManifestWorkActions: noAction,
			validateDynamicActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 2 {
					t.Fatal(spew.Sdump(actions))
				}

				action := actions[0].(clienttesting.GetAction)
				resource, namespace, name := action.GetResource(), action.GetNamespace(), action.GetName()
				if !reflect.DeepEqual(resource, schema.GroupVersionResource{Version: "v1", Resource: "secrets"}) || namespace != "ns1" || name != "n1" {
					t.Fatal(spew.Sdump(actions[0]))
				}
				action = actions[1].(clienttesting.GetAction)
				resource, namespace, name = action.GetResource(), action.GetNamespace(), action.GetName()
				if !reflect.DeepEqual(resource, schema.GroupVersionResource{Version: "v1", Resource: "secrets"}) || namespace != "ns2" || name != "n2" {
					t.Fatal(spew.Sdump(actions[0]))
				}
			},
			expectedQueueLen: 1,
		},
		{
			name:               "ignore re-created resource and remove finalizer",
			terminated:         true,
			existingFinalizers: []string{controllers.AppliedManifestWorkFinalizer},
			existingResources: []runtime.Object{
				spoketesting.NewUnstructuredSecret("ns1", "n1", false, "ns1-n1"),
			},
			resourcesToRemove: []workapiv1.AppliedManifestResourceMeta{
				{Version: "v1", Resource: "secrets", Namespace: "ns1", Name: "n1", UID: "n1"},
				{Version: "v1", Resource: "secrets", Namespace: "ns2", Name: "n2", UID: "n2"},
			},
			validateAppliedManifestWorkActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 2 {
					t.Fatal(spew.Sdump(actions))
				}

				work := actions[0].(clienttesting.UpdateAction).GetObject().(*workapiv1.AppliedManifestWork)
				if len(work.Status.AppliedResources) != 0 {
					t.Fatal(spew.Sdump(actions[0]))
				}

				work = actions[1].(clienttesting.UpdateAction).GetObject().(*workapiv1.AppliedManifestWork)
				if !reflect.DeepEqual(work.Finalizers, []string{}) {
					t.Fatal(spew.Sdump(actions[0]))
				}
			},
			validateDynamicActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 2 {
					t.Fatal(spew.Sdump(actions))
				}

				action := actions[0].(clienttesting.GetAction)
				resource, namespace, name := action.GetResource(), action.GetNamespace(), action.GetName()
				if !reflect.DeepEqual(resource, schema.GroupVersionResource{Version: "v1", Resource: "secrets"}) || namespace != "ns1" || name != "n1" {
					t.Fatal(spew.Sdump(actions[0]))
				}

				action = actions[1].(clienttesting.GetAction)
				resource, namespace, name = action.GetResource(), action.GetNamespace(), action.GetName()
				if !reflect.DeepEqual(resource, schema.GroupVersionResource{Version: "v1", Resource: "secrets"}) || namespace != "ns2" || name != "n2" {
					t.Fatal(spew.Sdump(actions[0]))
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testingWork := spoketesting.NewAppliedManifestWork("test", 0)
			testingWork.Finalizers = c.existingFinalizers
			if c.terminated {
				now := metav1.Now()
				testingWork.DeletionTimestamp = &now
			}
			for _, curr := range c.resourcesToRemove {
				testingWork.Status.AppliedResources = append(testingWork.Status.AppliedResources, curr)
			}

			fakeDynamicClient := fakedynamic.NewSimpleDynamicClient(runtime.NewScheme(), c.existingResources...)
			fakeClient := fakeworkclient.NewSimpleClientset(testingWork)
			controller := AppliedManifestWorkFinalizeController{
				appliedManifestWorkClient: fakeClient.WorkV1().AppliedManifestWorks(),
				spokeDynamicClient:        fakeDynamicClient,
				rateLimiter:               workqueue.NewItemExponentialFailureRateLimiter(0, 1*time.Second),
			}

			controllerContext := spoketesting.NewFakeSyncContext(t, testingWork.Name)
			err := controller.syncAppliedManifestWork(context.TODO(), controllerContext, testingWork)
			if err != nil {
				t.Fatal(err)
			}
			c.validateAppliedManifestWorkActions(t, fakeClient.Actions())
			c.validateDynamicActions(t, fakeDynamicClient.Actions())

			queueLen := controllerContext.Queue().Len()
			if queueLen != c.expectedQueueLen {
				t.Errorf("expected %d, but %d", c.expectedQueueLen, queueLen)
			}
		})
	}
}

func noAction(t *testing.T, actions []clienttesting.Action) {
	if len(actions) > 0 {
		t.Fatal(spew.Sdump(actions))
	}
}
