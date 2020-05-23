package framework

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"
	consoleapi "github.com/openshift/console-operator/pkg/api"
)

var (
	// AsyncOperationTimeout is how long we want to wait for asynchronous
	// operations to complete. ForeverTestTimeout is not long enough to create
	// several replicas and get them available on a slow machine.
	// Setting this to 5 minutes:w

	AsyncOperationTimeout = 5 * time.Minute
)

type TestFramework struct {
	ctx context.Context
}

type TestingResource struct {
	kind      string
	name      string
	namespace string
}

func getTestingResources() []TestingResource {
	return []TestingResource{
		{"ConfigMap", consoleapi.OpenShiftConsoleConfigMapName, consoleapi.OpenShiftConsoleNamespace},
		{"ConsoleCLIDownloads", consoleapi.OCCLIDownloadsCustomResourceName, ""},
		{"ConsoleCLIDownloads", consoleapi.ODOCLIDownloadsCustomResourceName, ""},
		{"Deployment", consoleapi.OpenShiftConsoleDeploymentName, consoleapi.OpenShiftConsoleNamespace},
		{"Route", consoleapi.OpenShiftConsoleRouteName, consoleapi.OpenShiftConsoleNamespace},
		{"Service", consoleapi.OpenShiftConsoleServiceName, consoleapi.OpenShiftConsoleNamespace},
	}
}

func SetClusterProxyConfig(proxyConfig configv1.ProxySpec, client *ClientSet) error {
	_, err := client.Proxy.Proxies().Patch(context.TODO(), consoleapi.ConfigResourceName, types.MergePatchType, []byte(fmt.Sprintf(`{"spec": {"httpProxy": "%s", "httpsProxy": "%s", "noProxy": "%s"}}`, proxyConfig.HTTPProxy, proxyConfig.HTTPSProxy, proxyConfig.NoProxy)), metav1.PatchOptions{})
	return err
}

func ResetClusterProxyConfig(client *ClientSet) error {
	_, err := client.Proxy.Proxies().Patch(context.TODO(), consoleapi.ConfigResourceName, types.MergePatchType, []byte(`{"spec": {"httpProxy": "", "httpsProxy": "", "noProxy": ""}}`), metav1.PatchOptions{})
	return err
}

func DeleteAll(t *testing.T, client *ClientSet) {
	resources := getTestingResources()

	for _, resource := range resources {
		t.Logf("deleting console's %s %s...", resource.name, resource.kind)
		if err := DeleteCompletely(
			func() (runtime.Object, error) {
				return GetResource(client, resource)
			},
			func(*metav1.DeleteOptions) error {
				return deleteResource(client, resource)
			},
		); err != nil {
			t.Fatalf("unable to delete console's %s %s: %s", resource.name, resource.kind, err)
		}
	}
}

func GetResource(client *ClientSet, resource TestingResource) (runtime.Object, error) {
	var res runtime.Object
	var err error

	switch resource.kind {
	case "ConfigMap":
		res, err = client.Core.ConfigMaps(resource.namespace).Get(context.TODO(), resource.name, metav1.GetOptions{})
	case "Service":
		res, err = client.Core.Services(resource.namespace).Get(context.TODO(), resource.name, metav1.GetOptions{})
	case "Route":
		res, err = client.Routes.Routes(resource.namespace).Get(context.TODO(), resource.name, metav1.GetOptions{})
	case "ConsoleCLIDownloads":
		res, err = client.ConsoleCliDownloads.Get(context.TODO(), resource.name, metav1.GetOptions{})
	case "Deployment":
		res, err = client.Apps.Deployments(resource.namespace).Get(context.TODO(), resource.name, metav1.GetOptions{})
	default:
		err = fmt.Errorf("error getting resource: resource %s not identified", resource.kind)
	}
	return res, err
}

// custom-logo in openshift-console should exist when custom branding is used
func GetCustomLogoConfigMap(client *ClientSet) (*corev1.ConfigMap, error) {
	return client.Core.ConfigMaps(consoleapi.OpenShiftConsoleNamespace).Get(context.TODO(), consoleapi.OpenShiftCustomLogoConfigMapName, metav1.GetOptions{})
}

func GetConsoleConfigMap(client *ClientSet) (*corev1.ConfigMap, error) {
	return client.Core.ConfigMaps(consoleapi.OpenShiftConsoleNamespace).Get(context.TODO(), consoleapi.OpenShiftConsoleConfigMapName, metav1.GetOptions{})
}

func GetConsoleService(client *ClientSet) (*corev1.Service, error) {
	return client.Core.Services(consoleapi.OpenShiftConsoleNamespace).Get(context.TODO(), consoleapi.OpenShiftConsoleServiceName, metav1.GetOptions{})
}

func GetConsoleRoute(client *ClientSet) (*routev1.Route, error) {
	return client.Routes.Routes(consoleapi.OpenShiftConsoleNamespace).Get(context.TODO(), consoleapi.OpenShiftConsoleRouteName, metav1.GetOptions{})
}

func GetConsoleDeployment(client *ClientSet) (*appv1.Deployment, error) {
	return client.Apps.Deployments(consoleapi.OpenShiftConsoleNamespace).Get(context.TODO(), consoleapi.OpenShiftConsoleDeploymentName, metav1.GetOptions{})
}

func GetConsoleCLIDownloads(client *ClientSet, consoleCLIDownloadName string) (*consolev1.ConsoleCLIDownload, error) {
	return client.ConsoleCliDownloads.Get(context.TODO(), consoleCLIDownloadName, metav1.GetOptions{})
}

func deleteResource(client *ClientSet, resource TestingResource) error {
	var err error
	switch resource.kind {
	case "ConfigMap":
		err = client.Core.ConfigMaps(resource.namespace).Delete(context.TODO(), resource.name, metav1.DeleteOptions{})
	case "Service":
		err = client.Core.Services(resource.namespace).Delete(context.TODO(), resource.name, metav1.DeleteOptions{})
	case "Route":
		err = client.Routes.Routes(resource.namespace).Delete(context.TODO(), resource.name, metav1.DeleteOptions{})
	case "ConsoleCLIDownloads":
		err = client.ConsoleCliDownloads.Delete(context.TODO(), resource.name, metav1.DeleteOptions{})
	case "Deployment":
		err = client.Apps.Deployments(resource.namespace).Delete(context.TODO(), resource.name, metav1.DeleteOptions{})
	default:
		err = fmt.Errorf("error deleting resource: resource %s not identified", resource.kind)
	}
	return err
}

// DeleteCompletely sends a delete request and waits until the resource and
// its dependents are deleted.
func DeleteCompletely(getObject func() (runtime.Object, error), deleteObject func(*metav1.DeleteOptions) error) error {
	obj, err := getObject()
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	accessor, err := meta.Accessor(obj)
	uid := accessor.GetUID()

	policy := metav1.DeletePropagationForeground
	if err := deleteObject(&metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{
			UID: &uid,
		},
		PropagationPolicy: &policy,
	}); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return wait.Poll(1*time.Second, AsyncOperationTimeout, func() (stop bool, err error) {
		obj, err = getObject()
		if err != nil {
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}

		accessor, err := meta.Accessor(obj)

		return accessor.GetUID() != uid, nil
	})
}

// atomicFailure supports reporting whether one or more of a set of goroutines
// has a failure condition to report.
type atomicFailure struct {
	sync.Mutex
	failed bool
}

func (a *atomicFailure) setFailed() {
	a.Lock()
	defer a.Unlock()
	a.failed = true
}

func (a *atomicFailure) getFailed() bool {
	a.Lock()
	defer a.Unlock()
	return a.failed
}

// CheckConsoleResourcesAvailable checks if the console resources are available
// and stay that way for at least 30s.
func CheckConsoleResourcesAvailable(t *testing.T, client *ClientSet) {
	resources := getTestingResources()
	// We have to test the `console-public` configmap in the TestManaged as well.
	resources = append(resources, TestingResource{"ConfigMap", consoleapi.OpenShiftConsolePublicConfigMapName, consoleapi.OpenShiftConfigManagedNamespace})

	failed := atomicFailure{}
	var wg sync.WaitGroup
	for _, resource := range resources {
		wg.Add(1)
		go func(resource TestingResource) {
			if !IsResourceAvailable(t, client, resource) {
				failed.setFailed()
			}
		}(resource)
	}
	wg.Wait()
	if failed.getFailed() {
		t.Fatalf("One or more console resources did not stay available for at least 30s.")
	}
}

// IsResourceAvailable indicates whether the resource are available and stay
// that way for at least 30s. Once a resource becomes available, it should not
// go missing.
func IsResourceAvailable(t *testing.T, client *ClientSet, resource TestingResource) bool {
	startTime := time.Now()
	minimumDuration := time.Second * 30
	err := wait.Poll(1*time.Second, AsyncOperationTimeout, func() (stop bool, err error) {
		_, err = GetResource(client, resource)
		if errors.IsNotFound(err) {
			// Resource does not exist. Reset timer and keep waiting.
			startTime = time.Now()
			return false, nil
		}
		if err != nil {
			// Unable to retrieve resource due to error. Reset timer and keep waiting.
			startTime = time.Now()
			t.Errorf("Error checking that console %s %s exists: %v", resource.kind, resource.name, err)
			return false, nil
		}
		minimumDurationElapsed := startTime.Add(minimumDuration).Before(time.Now())
		return minimumDurationElapsed, nil
	})
	if err != nil {
		t.Errorf("Timed out waiting for console %s %s to exist for at least %v", resource.kind, resource.name, minimumDuration)
	}
	return err == nil
}

// CheckConsoleResourcesUnavailable checks if the console resources go missing
// and stay that way for at least 30s.
func CheckConsoleResourcesUnavailable(t *testing.T, client *ClientSet) {
	resources := getTestingResources()

	failed := atomicFailure{}
	var wg sync.WaitGroup
	for _, resource := range resources {
		wg.Add(1)
		go func(resource TestingResource) {
			if IsResourceUnavailable(t, client, resource) {
				failed.setFailed()
			}
		}(resource)
	}
	wg.Wait()
	if failed.getFailed() {
		t.Fatalf("One or more console resources did not stay missing for at least 30s.")
	}
}

// IsResourceUnavailable indicates whether the console resource goes missing and
// stays that way for at least 30s. Once a resource is missing, it should not
// reappear.
func IsResourceUnavailable(t *testing.T, client *ClientSet, resource TestingResource) bool {
	startTime := time.Now()
	minimumDuration := time.Second * 30
	err := wait.PollImmediate(1*time.Second, AsyncOperationTimeout, func() (stop bool, err error) {
		_, err = GetResource(client, resource)
		if err == nil {
			// Resource exists. Reset timer and keep waiting.
			startTime = time.Now()
			t.Logf("Console %s %s exists", resource.kind, resource.name)
			return false, nil
		}
		if !errors.IsNotFound(err) {
			// Unable to retrieve resource due to error. Reset timer and keep waiting.
			startTime = time.Now()
			t.Errorf("Error checking that console %s %s exists: %v", resource.kind, resource.name, err)
			return false, nil
		}
		// Resource is missing. Exit if the minimum duration has elapsed.
		minimumDurationElapsed := startTime.Add(minimumDuration).Before(time.Now())
		return minimumDurationElapsed, nil
	})
	if err != nil {
		t.Errorf("timed out waiting for console %s %s to be missing for at least %v", resource.kind, resource.name, minimumDuration)
	}
	return err == nil
}
