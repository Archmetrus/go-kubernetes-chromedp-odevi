package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s-chromedp-synthetics/pkg/diff"
	"k8s-chromedp-synthetics/pkg/metrics"
	"k8s-chromedp-synthetics/pkg/runner"
	"k8s-chromedp-synthetics/pkg/store"
	"k8s-chromedp-synthetics/pkg/types"
)

var SyntheticTestGVR = schema.GroupVersionResource{
	Group:    "monitoring.k8s.io",
	Version:  "v1alpha1",
	Resource: "synthetictests",
}

type SyntheticController struct {
	DynamicClient dynamic.Interface
	Runner        *runner.ChromeRunner
	Storage       *store.StorageManager
	activeTasks   sync.Map
}

func NewSyntheticController(r *runner.ChromeRunner, sm *store.StorageManager) (*SyntheticController, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to local kubeconfig for out-of-cluster dev
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			home, _ := os.UserHomeDir()
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
		var errConfig error
		config, errConfig = rest.InClusterConfig()
		if errConfig != nil {
			log.Printf("Notice: Running in standalone mode without K8s cluster connection (%v)", errConfig)
			return &SyntheticController{
				Runner:  r,
				Storage: sm,
			}, nil
		}
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &SyntheticController{
		DynamicClient: dynClient,
		Runner:        r,
		Storage:       sm,
	}, nil
}

// StartInformer starts watching SyntheticTest CRDs in the Kubernetes cluster
func (c *SyntheticController) StartInformer(ctx context.Context) {
	if c.DynamicClient == nil {
		log.Println("K8s Informer skipped: DynamicClient is nil (Standalone Mode)")
		return
	}

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(c.DynamicClient, 30*time.Second, metav1.NamespaceAll, nil)
	informer := factory.ForResource(SyntheticTestGVR).Informer()

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			log.Printf("[K8s Controller] SyntheticTest Added: %s/%s", u.GetNamespace(), u.GetName())
			c.reconcileSyntheticTest(ctx, u)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			u := newObj.(*unstructured.Unstructured)
			log.Printf("[K8s Controller] SyntheticTest Updated: %s/%s", u.GetNamespace(), u.GetName())
			c.reconcileSyntheticTest(ctx, u)
		},
		DeleteFunc: func(obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			log.Printf("[K8s Controller] SyntheticTest Deleted: %s/%s", u.GetNamespace(), u.GetName())
		},
	})

	log.Println("[K8s Controller] Starting Informer for SyntheticTest CRDs...")
	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())
}

func (c *SyntheticController) reconcileSyntheticTest(ctx context.Context, u *unstructured.Unstructured) {
	name := u.GetName()
	namespace := u.GetNamespace()
	testID := fmt.Sprintf("%s-%s", namespace, name)

	// Extract Spec
	specMap, found, err := unstructured.NestedMap(u.Object, "spec")
	if !found || err != nil {
		log.Printf("Error extracting spec from %s: %v", testID, err)
		return
	}

	specBytes, _ := json.Marshal(specMap)
	var spec types.SyntheticTestSpec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		log.Printf("Error parsing SyntheticTestSpec for %s: %v", testID, err)
		return
	}

	// Update status to Running
	c.updateCRDStatus(ctx, namespace, name, "Running", nil, 0, "")

	// Execute Test
	startTime := time.Now()
	results, screenshotBytes, err := c.Runner.ExecuteScenario(ctx, testID, spec)
	durSeconds := time.Since(startTime).Seconds()

	phase := "Succeeded"
	var lastErrMsg string
	if err != nil {
		phase = "Failed"
		lastErrMsg = err.Error()
	}

	var driftPercent *float64
	if len(screenshotBytes) > 0 {
		_, fullPath, saveErr := c.Storage.SaveScreenshot(testID, screenshotBytes)
		if saveErr == nil {
			baselinePath, hasBaseline := c.Storage.GetBaseline(testID)
			if hasBaseline && spec.VisualRegression {
				diffRes, diffBytes, compErr := diff.CompareImages(baselinePath, fullPath, spec.DriftThreshold)
				if compErr == nil {
					driftPercent = &diffRes.DiffPercent
					c.Storage.SaveDiffImage(testID, diffBytes)
				}
			} else {
				c.Storage.SaveBaseline(testID, screenshotBytes)
			}
		}
	}

	// Record Prometheus Metrics
	metrics.RecordRun(testID, phase, durSeconds, driftPercent)

	// Update status to K8s CRD
	var driftVal float64
	if driftPercent != nil {
		driftVal = *driftPercent
	}
	c.updateCRDStatus(ctx, namespace, name, phase, results, driftVal, lastErrMsg)
}

func (c *SyntheticController) updateCRDStatus(ctx context.Context, namespace, name, phase string, stepResults []types.StepResult, visualDrift float64, errMsg string) {
	if c.DynamicClient == nil {
		return
	}

	u, err := c.DynamicClient.Resource(SyntheticTestGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error fetching CRD for status update: %v", err)
		return
	}

	statusMap := map[string]interface{}{
		"phase":            phase,
		"lastRunTime":      time.Now().Format(time.RFC3339),
		"visualDrift":      visualDrift,
		"lastErrorMessage": errMsg,
	}

	if stepResults != nil {
		statusMap["stepResults"] = stepResults
	}

	unstructured.SetNestedMap(u.Object, statusMap, "status")
	_, err = c.DynamicClient.Resource(SyntheticTestGVR).Namespace(namespace).UpdateStatus(ctx, u, metav1.UpdateOptions{})
	if err != nil {
		log.Printf("Failed to update status for SyntheticTest %s/%s: %v", namespace, name, err)
	} else {
		log.Printf("Updated Status for SyntheticTest %s/%s -> Phase: %s", namespace, name, phase)
	}
}
