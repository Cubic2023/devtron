/*
 * Copyright (c) 2020 Devtron Labs
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package util

import (
	"context"
	"encoding/json"
	error2 "errors"
	"flag"
	"fmt"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/argoproj/gitops-engine/pkg/utils/kube"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"os/user"
	"path/filepath"
	"strconv"
	"time"

	"github.com/devtron-labs/authenticator/client"
	"github.com/ghodss/yaml"
	"go.uber.org/zap"
	batchV1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	v12 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type K8sUtil struct {
	logger        *zap.SugaredLogger
	runTimeConfig *client.RuntimeConfig
	kubeconfig    *string
}

type ClusterConfig struct {
	Host        string
	BearerToken string
}

func NewK8sUtil(logger *zap.SugaredLogger, runTimeConfig *client.RuntimeConfig) *K8sUtil {
	usr, err := user.Current()
	if err != nil {
		return nil
	}
	var kubeconfig *string
	if runTimeConfig.LocalDevMode {
		kubeconfig = flag.String("kubeconfig-authenticator-xyz", filepath.Join(usr.HomeDir, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	}

	flag.Parse()
	return &K8sUtil{logger: logger, runTimeConfig: runTimeConfig, kubeconfig: kubeconfig}
}

func (impl K8sUtil) GetClient(clusterConfig *ClusterConfig) (*v12.CoreV1Client, error) {
	cfg := &rest.Config{}
	cfg.Host = clusterConfig.Host
	cfg.BearerToken = clusterConfig.BearerToken
	cfg.Insecure = true
	client, err := v12.NewForConfig(cfg)
	return client, err
}

func (impl K8sUtil) GetClientSet(clusterConfig *ClusterConfig) (*kubernetes.Clientset, error) {
	cfg := &rest.Config{}
	cfg.Host = clusterConfig.Host
	cfg.BearerToken = clusterConfig.BearerToken
	cfg.Insecure = true
	client, err := kubernetes.NewForConfig(cfg)
	return client, err
}

func (impl K8sUtil) getKubeConfig(devMode client.LocalDevMode) (*rest.Config, error) {
	if devMode {
		restConfig, err := clientcmd.BuildConfigFromFlags("", *impl.kubeconfig)
		if err != nil {
			return nil, err
		}
		return restConfig, nil
	} else {
		restConfig, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		return restConfig, nil
	}
}

func (impl K8sUtil) GetClientForInCluster() (*v12.CoreV1Client, error) {
	// creates the in-cluster config
	config, err := impl.getKubeConfig(impl.runTimeConfig.LocalDevMode)
	// creates the clientset
	clientset, err := v12.NewForConfig(config)
	if err != nil {
		impl.logger.Errorw("error", "error", err)
		return nil, err
	}
	return clientset, err
}

func (impl K8sUtil) GetK8sClient() (*v12.CoreV1Client, error) {
	var config *rest.Config
	var err error
	if impl.runTimeConfig.LocalDevMode {
		config, err = clientcmd.BuildConfigFromFlags("", *impl.kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		impl.logger.Errorw("error fetching cluster config", "error", err)
		return nil, err
	}
	client, err := v12.NewForConfig(config)
	if err != nil {
		impl.logger.Errorw("error creating k8s client", "error", err)
		return nil, err
	}
	return client, err
}

func (impl K8sUtil) GetK8sDiscoveryClient(clusterConfig *ClusterConfig) (*discovery.DiscoveryClient, error) {
	cfg := &rest.Config{}
	cfg.Host = clusterConfig.Host
	cfg.BearerToken = clusterConfig.BearerToken
	cfg.Insecure = true
	client, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		impl.logger.Errorw("error", "error", err, "clusterConfig", clusterConfig)
		return nil, err
	}
	return client, err
}

func (impl K8sUtil) GetK8sDiscoveryClientInCluster() (*discovery.DiscoveryClient, error) {
	var config *rest.Config
	var err error
	if impl.runTimeConfig.LocalDevMode {
		config, err = clientcmd.BuildConfigFromFlags("", *impl.kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}

	if err != nil {
		impl.logger.Errorw("error", "error", err)
		return nil, err
	}
	client, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		impl.logger.Errorw("error", "error", err)
		return nil, err
	}
	return client, err
}

func (impl K8sUtil) CreateNsIfNotExists(namespace string, clusterConfig *ClusterConfig) (err error) {
	client, err := impl.GetClient(clusterConfig)
	if err != nil {
		impl.logger.Errorw("error", "error", err, "clusterConfig", clusterConfig)
		return err
	}
	exists, err := impl.checkIfNsExists(namespace, client)
	if err != nil {
		impl.logger.Errorw("error", "error", err, "clusterConfig", clusterConfig)
		return err
	}
	if exists {
		return nil
	}
	impl.logger.Infow("ns not exists creating", "ns", namespace)
	_, err = impl.createNs(namespace, client)
	return err
}

func (impl K8sUtil) checkIfNsExists(namespace string, client *v12.CoreV1Client) (exists bool, err error) {
	ns, err := client.Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})
	//ns, err := impl.k8sClient.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	impl.logger.Debugw("ns fetch", "name", namespace, "res", ns)
	if errors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	} else {
		return true, nil
	}

}

func (impl K8sUtil) createNs(namespace string, client *v12.CoreV1Client) (ns *v1.Namespace, err error) {
	nsSpec := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	ns, err = client.Namespaces().Create(context.Background(), nsSpec, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	} else {
		return ns, nil
	}
}

func (impl K8sUtil) deleteNs(namespace string, client *v12.CoreV1Client) error {
	err := client.Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	return err
}

func (impl K8sUtil) GetConfigMap(namespace string, name string, client *v12.CoreV1Client) (*v1.ConfigMap, error) {
	cm, err := client.ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	} else {
		return cm, nil
	}
}

func (impl K8sUtil) CreateConfigMap(namespace string, cm *v1.ConfigMap, client *v12.CoreV1Client) (*v1.ConfigMap, error) {
	cm, err := client.ConfigMaps(namespace).Create(context.Background(), cm, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	} else {
		return cm, nil
	}
}

func (impl K8sUtil) UpdateConfigMap(namespace string, cm *v1.ConfigMap, client *v12.CoreV1Client) (*v1.ConfigMap, error) {
	cm, err := client.ConfigMaps(namespace).Update(context.Background(), cm, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	} else {
		return cm, nil
	}
}

func (impl K8sUtil) PatchConfigMap(namespace string, clusterConfig *ClusterConfig, name string, data map[string]interface{}) (*v1.ConfigMap, error) {
	client, err := impl.GetClient(clusterConfig)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	cm, err := client.ConfigMaps(namespace).Patch(context.Background(), name, types.PatchType(types.MergePatchType), b, metav1.PatchOptions{})
	if err != nil {
		return nil, err
	} else {
		return cm, nil
	}
	return cm, nil
}

func (impl K8sUtil) PatchConfigMapJsonType(namespace string, clusterConfig *ClusterConfig, name string, data interface{}, path string) (*v1.ConfigMap, error) {
	client, err := impl.GetClient(clusterConfig)
	if err != nil {
		return nil, err
	}
	var patches []*JsonPatchType
	patch := &JsonPatchType{
		Op:    "replace",
		Path:  path,
		Value: data,
	}
	patches = append(patches, patch)
	b, err := json.Marshal(patches)
	if err != nil {
		panic(err)
	}

	cm, err := client.ConfigMaps(namespace).Patch(context.Background(), name, types.PatchType(types.JSONPatchType), b, metav1.PatchOptions{})
	if err != nil {
		return nil, err
	} else {
		return cm, nil
	}
	return cm, nil
}

type JsonPatchType struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

func (impl K8sUtil) GetSecret(namespace string, name string, client *v12.CoreV1Client) (*v1.Secret, error) {
	secret, err := client.Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	} else {
		return secret, nil
	}
}

func (impl K8sUtil) CreateSecret(namespace string, data map[string][]byte, secretName string, secretType v1.SecretType, client *v12.CoreV1Client) (*v1.Secret, error) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
		Data: data,
	}
	if len(secretType) > 0 {
		secret.Type = secretType
	}
	secret, err := client.Secrets(namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	} else {
		return secret, nil
	}
}

func (impl K8sUtil) UpdateSecret(namespace string, secret *v1.Secret, client *v12.CoreV1Client) (*v1.Secret, error) {
	secret, err := client.Secrets(namespace).Update(context.Background(), secret, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	} else {
		return secret, nil
	}
}

func (impl K8sUtil) DeleteJob(namespace string, name string, clusterConfig *ClusterConfig) error {
	clientSet, err := impl.GetClientSet(clusterConfig)
	if err != nil {
		impl.logger.Errorw("clientSet err, DeleteJob", "err", err)
		return err
	}
	jobs := clientSet.BatchV1().Jobs(namespace)

	job, err := jobs.Get(context.Background(), name, metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) {
		impl.logger.Errorw("get job err, DeleteJob", "err", err)
		return nil
	}

	if job != nil {
		err := jobs.Delete(context.Background(), name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			impl.logger.Errorw("delete err, DeleteJob", "err", err)
			return err
		}
	}

	return nil
}

func (impl K8sUtil) CreateJob(namespace string, name string, clusterConfig *ClusterConfig, job *batchV1.Job) error {
	clientSet, err := impl.GetClientSet(clusterConfig)
	if err != nil {
		impl.logger.Errorw("clientSet err, CreateJob", "err", err)
	}
	time.Sleep(5 * time.Second)

	jobs := clientSet.BatchV1().Jobs(namespace)
	_, err = jobs.Get(context.Background(), name, metav1.GetOptions{})
	if err == nil {
		impl.logger.Errorw("get job err, CreateJob", "err", err)
		time.Sleep(5 * time.Second)
		_, err = jobs.Get(context.Background(), name, metav1.GetOptions{})
		if err == nil {
			return error2.New("job deletion takes more time than expected, please try after sometime")
		}
	}

	_, err = jobs.Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		impl.logger.Errorw("create err, CreateJob", "err", err)
		return err
	}
	return nil
}

// DeletePod delete pods with label job-name

const Running = "Running"

func (impl K8sUtil) DeletePodByLabel(namespace string, labels string, clusterConfig *ClusterConfig) error {
	clientSet, err := impl.GetClientSet(clusterConfig)
	if err != nil {
		impl.logger.Errorw("clientSet err, DeletePod", "err", err)
		return err
	}

	time.Sleep(2 * time.Second)

	pods := clientSet.CoreV1().Pods(namespace)
	podList, err := pods.List(context.Background(), metav1.ListOptions{LabelSelector: labels})
	if err != nil && errors.IsNotFound(err) {
		impl.logger.Errorw("get pod err, DeletePod", "err", err)
		return nil
	}

	for _, pod := range (*podList).Items {
		if pod.Status.Phase != Running {
			podName := pod.ObjectMeta.Name
			err := pods.Delete(context.Background(), podName, metav1.DeleteOptions{})
			if err != nil && !errors.IsNotFound(err) {
				impl.logger.Errorw("delete err, DeletePod", "err", err)
				return err
			}
		}
	}
	return nil
}

// DeleteAndCreateJob Deletes and recreates if job exists else creates the job
func (impl K8sUtil) DeleteAndCreateJob(content []byte, namespace string, clusterConfig *ClusterConfig) error {
	// Job object from content
	var job batchV1.Job
	err := yaml.Unmarshal(content, &job)
	if err != nil {
		impl.logger.Errorw("Unmarshal err, CreateJobSafely", "err", err)
		return err
	}

	// delete job if exists
	err = impl.DeleteJob(namespace, job.Name, clusterConfig)
	if err != nil {
		impl.logger.Errorw("DeleteJobIfExists err, CreateJobSafely", "err", err)
		return err
	}

	labels := "job-name=" + job.Name
	err = impl.DeletePodByLabel(namespace, labels, clusterConfig)
	if err != nil {
		impl.logger.Errorw("DeleteJobIfExists err, CreateJobSafely", "err", err)
		return err
	}
	// create job
	err = impl.CreateJob(namespace, job.Name, clusterConfig, &job)
	if err != nil {
		impl.logger.Errorw("CreateJob err, CreateJobSafely", "err", err)
		return err
	}

	return nil
}

func (impl K8sUtil) ListNamespaces(client *v12.CoreV1Client) (*v1.NamespaceList, error) {
	nsList, err := client.Namespaces().List(context.Background(), metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nsList, nil
	} else if err != nil {
		return nsList, err
	} else {
		return nsList, nil
	}
}

func (impl K8sUtil) GetClientByToken(serverUrl string, token map[string]string) (*v12.CoreV1Client, error) {
	bearerToken := token["bearer_token"]
	clusterCfg := &ClusterConfig{Host: serverUrl, BearerToken: bearerToken}
	client, err := impl.GetClient(clusterCfg)
	if err != nil {
		impl.logger.Errorw("error in k8s client", "error", err)
		return nil, err
	}
	return client, nil
}

func (impl K8sUtil) GetResourceInfoByLabelSelector(namespace string, labelSelector string) (*v1.Pod, error) {
	client, err := impl.GetClientForInCluster()
	if err != nil {
		impl.logger.Errorw("cluster config error", "err", err)
		return nil, err
	}
	pods, err := client.Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	} else if len(pods.Items) > 1 {
		err = &ApiError{Code: "406", HttpStatusCode: 200, UserMessage: "found more than one pod for label selector"}
		return nil, err
	} else if len(pods.Items) == 0 {
		err = &ApiError{Code: "404", HttpStatusCode: 200, UserMessage: "no pod found for label selector"}
		return nil, err
	} else {
		return &pods.Items[0], nil
	}
}

func (impl K8sUtil) GetK8sClusterRestConfig() (*rest.Config, error) {
	impl.logger.Debug("getting k8s rest config")
	if impl.runTimeConfig.LocalDevMode {
		usr, err := user.Current()
		if err != nil {
			impl.logger.Errorw("Error while getting user current env details", "error", err)
		}
		kubeconfig := flag.String("read-kubeconfig", filepath.Join(usr.HomeDir, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		flag.Parse()
		restConfig, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			impl.logger.Errorw("Error while building kubernetes cluster rest config", "error", err)
			return nil, err
		}
		return restConfig, nil
	} else {
		clusterConfig, err := rest.InClusterConfig()
		if err != nil {
			impl.logger.Errorw("error in fetch default cluster config", "err", err)
			return nil, err
		}
		return clusterConfig, nil
	}
}

func (impl K8sUtil) GetPodByName(namespace string, name string, client *v12.CoreV1Client) (*v1.Pod, error) {
	pod, err := client.Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		impl.logger.Errorw("error in fetch pod name", "err", err)
		return nil, err
	} else {
		return pod, nil
	}
}

// ParseResource TODO - optimize and refactor, WIP
func (impl K8sUtil) ParseResource(manifest *unstructured.Unstructured) (map[string]string, error) {
	clusterResourceListResponse := make(map[string]string)

	switch manifest.GroupVersionKind() {
	case schema.GroupVersionKind{Group: "", Version: "v1", Kind: kube.PodKind}:
		var pod v1.Pod
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &pod)
		if err != nil {
			return nil, err
		}
		clusterResourceListResponse = impl.populatePodData(pod)
		/*	case schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kube.DeploymentKind}:
				var deployment v1beta2.Deployment
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &deployment)
				if err != nil {
					return nil, err
				}
				clusterResourceListResponse["name"] = deployment.Name
				clusterResourceListResponse["namespace"] = deployment.Namespace
				clusterResourceListResponse["status"] = ""
			case schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kube.ReplicaSetKind}:
				var replicaSet v1beta2.ReplicaSet
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &replicaSet)
				if err != nil {
					return nil, err
				}
				clusterResourceListResponse["name"] = replicaSet.Name
				clusterResourceListResponse["namespace"] = replicaSet.Namespace
				clusterResourceListResponse["status"] = ""
			case schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kube.StatefulSetKind}:
				var statefulSet v1beta2.StatefulSet
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &statefulSet)
				if err != nil {
					return nil, err
				}
				clusterResourceListResponse["Name"] = statefulSet.Name
				clusterResourceListResponse["namespace"] = statefulSet.Namespace
				clusterResourceListResponse["status"] = ""
			case schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kube.DaemonSetKind}:
				var daemonSet v1beta2.DaemonSet
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &daemonSet)
				if err != nil {
					return nil, err
				}
				clusterResourceListResponse["name"] = daemonSet.Name
				clusterResourceListResponse["namespace"] = daemonSet.Namespace
				clusterResourceListResponse["status"] = ""
			case schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: kube.JobKind}:
				var job batchV1.Job
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &job)
				if err != nil {
					return nil, err
				}
				clusterResourceListResponse["name"] = job.Name
				clusterResourceListResponse["namespace"] = job.Namespace
				clusterResourceListResponse["status"] = ""
			case schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}:
				var cronJob batchV1.CronJob
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &cronJob)
				if err != nil {
					return nil, err
				}
				clusterResourceListResponse["name"] = cronJob.Name
				clusterResourceListResponse["namespace"] = cronJob.Namespace
				clusterResourceListResponse["status"] = ""
			case schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ReplicationController"}:
				var replicationController v1.ReplicationController
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &replicationController)
				if err != nil {
					return nil, err
				}
				clusterResourceListResponse["name"] = replicationController.Name
				clusterResourceListResponse["namespace"] = replicationController.Namespace
				clusterResourceListResponse["status"] = ""
			case schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Rollout"}:
				var rolloutSpec map[string]interface{}
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(manifest.UnstructuredContent(), &rolloutSpec)
				if err != nil {
					return nil, err
				}
				clusterResourceListResponse["name"] = rolloutSpec["name"].(string)
				clusterResourceListResponse["namespace"] = rolloutSpec["namespace"].(string)
				clusterResourceListResponse["status"] = ""*/
	default:
		clusterResourceListResponse = impl.populateOtherResourceData(manifest)
	}

	return clusterResourceListResponse, nil
}

func (impl K8sUtil) populatePodData(pod v1.Pod) map[string]string {
	clusterResourceListResponse := make(map[string]string)
	clusterResourceListResponse["name"] = pod.Name
	clusterResourceListResponse["namespace"] = pod.Namespace
	clusterResourceListResponse["age"] = pod.CreationTimestamp.String()
	clusterResourceListResponse["status"] = string(pod.Status.Phase)

	restarts := 0
	totalContainers := len(pod.Spec.Containers)
	readyContainers := 0
	for i := range pod.Status.InitContainerStatuses {
		container := pod.Status.InitContainerStatuses[i]
		restarts += int(container.RestartCount)
		if container.Ready {
			readyContainers += readyContainers
		}
	}

	clusterResourceListResponse["ready"] = fmt.Sprintf("%d/%d", readyContainers, totalContainers)
	clusterResourceListResponse["restarts"] = strconv.Itoa(restarts)
	return clusterResourceListResponse
}

func (impl K8sUtil) populateOtherResourceData(manifest *unstructured.Unstructured) map[string]string {
	clusterResourceListResponse := make(map[string]string)
	res := manifest.Object
	if res != nil && res["metadata"] != nil {
		metadata := res["metadata"].(map[string]interface{})
		clusterResourceListResponse["name"] = metadata["name"].(string)
		clusterResourceListResponse["namespace"] = metadata["namespace"].(string)
		clusterResourceListResponse["age"] = metadata["creationTimestamp"].(string)
	}

	if healthCheck := health.GetHealthCheckFunc(manifest.GroupVersionKind()); healthCheck != nil {
		health, err := healthCheck(manifest)
		if err != nil {
			impl.logger.Infow("error on health check for k8s resource", "err", err)
		} else if health != nil {
			clusterResourceListResponse["status"] = string(health.Status)
		}
	}
	return clusterResourceListResponse
}
