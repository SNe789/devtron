package argocdServer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/devtron-labs/devtron/pkg/cluster/repository"
	"go.uber.org/zap"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"path/filepath"
	"text/template"
	"time"
)

type AppTemplate struct {
	ApplicationName string
	Namespace       string
	TargetNamespace string
	TargetServer    string
	Project         string
	ValuesFile      string
	RepoPath        string
	RepoUrl         string
	TargetName      string
}

const TimeoutSlow = 30 * time.Second

type ArgoK8sClient interface {
	CreateAcdApp(appRequest *AppTemplate, cluster *repository.Cluster) (string, error)
	GetArgoApplication(namespace string, appName string, cluster *repository.Cluster) (map[string]interface{}, error)
}
type ArgoK8sClientImpl struct {
	logger *zap.SugaredLogger
}

func NewArgoK8sClientImpl(logger *zap.SugaredLogger,
) *ArgoK8sClientImpl {
	return &ArgoK8sClientImpl{
		logger: logger,
	}
}

const DevtronInstalationNs = "devtroncd"

// Tprintf passed template string is formatted usign its operands and returns the resulting string.
// Spaces are added between operands when neither is a string.
func (impl ArgoK8sClientImpl) tprintf(tmpl string, data interface{}) (string, error) {
	t := template.Must(template.New("tpl").Parse(tmpl))
	buf := &bytes.Buffer{}
	if err := t.Execute(buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (impl ArgoK8sClientImpl) CreateAcdApp(appRequest *AppTemplate, cluster *repository.Cluster) (string, error) {
	chartYamlContent, err := ioutil.ReadFile(filepath.Clean("./scripts/argo-assets/APPLICATION_TEMPLATE.JSON"))
	if err != nil {
		impl.logger.Errorw("err in reading template", "err", err)
		return "", err
	}
	applicationRequestString, err := impl.tprintf(string(chartYamlContent), appRequest)
	if err != nil {
		impl.logger.Errorw("error in rendering application template", "req", appRequest, "err", err)
		return "", err
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		impl.logger.Errorw("error in config", "err", err)
		return "", err
	}
	config.GroupVersion = &schema.GroupVersion{Group: "argoproj.io", Version: "v1alpha1"}
	config.NegotiatedSerializer = serializer.NewCodecFactory(runtime.NewScheme())
	config.APIPath = "/apis"
	config.Timeout = TimeoutSlow
	err = impl.CreateArgoApplication(appRequest.Namespace, applicationRequestString, config)
	if err != nil {
		impl.logger.Errorw("error in creating acd application", "err", err)
		return "", err
	}
	impl.logger.Infow("argo application created successfully", "name", appRequest.ApplicationName)
	return appRequest.ApplicationName, nil
}

func (impl ArgoK8sClientImpl) CreateArgoApplication(namespace string, application string, config *rest.Config) error {
	client, err := rest.RESTClientFor(config)
	if err != nil {
		return fmt.Errorf("error creating argo cd app")
	}
	impl.logger.Infow("creating application", "req", application)
	res, err := client.
		Post().
		Resource("applications").
		Namespace(namespace).
		Body([]byte(application)).
		Do(context.Background()).Raw()

	if err != nil {
		response := make(map[string]interface{})
		err := json.Unmarshal(res, &response)
		if err != nil {
			impl.logger.Errorw("unmarshal error on app update status", "err", err)
			return fmt.Errorf("error creating argo cd app")
		}
		message := "error creating argo cd app"
		if response != nil && response["message"] != nil {
			message = response["message"].(string)
		}
		return fmt.Errorf(message)
	}

	impl.logger.Infow("argo app create res", "res", string(res), "err", err)
	return err
}

func (impl ArgoK8sClientImpl) GetArgoApplication(namespace string, appName string, cluster *repository.Cluster) (map[string]interface{}, error) {

	config, err := rest.InClusterConfig()
	if err != nil {
		impl.logger.Errorw("error in cluster config", "err", err)
		return nil, err
	}
	config.GroupVersion = &schema.GroupVersion{Group: "argoproj.io", Version: "v1alpha1"}
	config.NegotiatedSerializer = serializer.NewCodecFactory(runtime.NewScheme())
	config.APIPath = "/apis"
	config.Timeout = TimeoutSlow
	client, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, err
	}
	impl.logger.Infow("get argo cd application", "req", appName)
	//acdApplication := &v1alpha12.Application{}
	//opts := metav1.GetOptions{}
	res, err := client.
		Get().
		Namespace(namespace).
		Resource("applications").
		Name(appName).
		//VersionedParams(&opts, metav1.ParameterCodec).
		Do(context.Background()).Raw()
	response := make(map[string]interface{})
	if err != nil {
		err := json.Unmarshal(res, &response)
		if err != nil {
			impl.logger.Errorw("unmarshal error on app update status", "err", err)
			return nil, fmt.Errorf("error get argo cd app")
		}
	}
	impl.logger.Infow("get argo cd application", "res", response, "err", err)
	return response, err
}
