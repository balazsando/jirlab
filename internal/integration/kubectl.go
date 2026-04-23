package integration

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// KubectlService abstracts all kubectl subprocess invocations so the TUI and
// business logic layers are not directly coupled to the kubectl binary.
type KubectlService interface {
	// GetResourceJSON runs `kubectl get <resource> --output=json` and returns the raw bytes.
	GetResourceJSON(configPath, resource string) ([]byte, error)
	// ScaleDeployment scales a deployment to the given replica count.
	ScaleDeployment(configPath, name string, replicas int) error
	// RunCommand runs an arbitrary kubectl sub-command and returns combined output.
	// args are passed directly after "kubectl". The KUBECONFIG env var is set to configPath.
	RunCommand(configPath string, args ...string) ([]byte, error)
}

type kubectlService struct{}

// NewKubectlService returns a KubectlService that invokes the local kubectl binary.
func NewKubectlService() KubectlService {
	return &kubectlService{}
}

func withConfig(configPath string) []string {
	env := make([]string, len(os.Environ()))
	copy(env, os.Environ())
	return append(env, "KUBECONFIG="+configPath)
}

func (k *kubectlService) GetResourceJSON(configPath, resource string) ([]byte, error) {
	cmd := exec.Command("kubectl", "get", resource, "--output=json") //nolint:gosec
	cmd.Env = withConfig(configPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("kubectl get %s: %w", resource, err)
	}
	return out, nil
}

func (k *kubectlService) ScaleDeployment(configPath, name string, replicas int) error {
	cmd := exec.Command("kubectl", "scale", "deployment", name, //nolint:gosec
		fmt.Sprintf("--replicas=%d", replicas))
	cmd.Env = withConfig(configPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scale %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (k *kubectlService) RunCommand(configPath string, args ...string) ([]byte, error) {
	cmd := exec.Command("kubectl", args...) //nolint:gosec
	cmd.Env = withConfig(configPath)
	return cmd.CombinedOutput()
}
