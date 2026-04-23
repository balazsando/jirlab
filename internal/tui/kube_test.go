package tui

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// podStatusColor
// ---------------------------------------------------------------------------

func TestPodStatusColorRunning(t *testing.T) {
	for _, s := range []string{"Running", "running"} {
		if c := podStatusColor(s); c != colorGreen {
			t.Errorf("podStatusColor(%q) = %v, want green", s, c)
		}
	}
}

func TestPodStatusColorCompleted(t *testing.T) {
	for _, s := range []string{"completed", "Completed", "succeeded", "Succeeded"} {
		if c := podStatusColor(s); c != colorSubtle {
			t.Errorf("podStatusColor(%q) = %v, want subtle (grey)", s, c)
		}
	}
}

func TestPodStatusColorPending(t *testing.T) {
	for _, s := range []string{"Pending", "ContainerCreating", "PodInitializing", "Terminating"} {
		if c := podStatusColor(s); c != colorYellow {
			t.Errorf("podStatusColor(%q) = %v, want yellow", s, c)
		}
	}
}

func TestPodStatusColorFailed(t *testing.T) {
	for _, s := range []string{"Failed", "CrashLoopBackOff", "Error", "OOMKilled",
		"ImagePullBackOff", "ErrImagePull", "InvalidImageName",
		"CreateContainerError", "RunInitError"} {
		if c := podStatusColor(s); c != colorRed {
			t.Errorf("podStatusColor(%q) = %v, want red", s, c)
		}
	}
}

func TestPodStatusColorHeuristicRed(t *testing.T) {
	for _, s := range []string{"SomeError", "CustomBackOff", "TaskFailed", "OomDetected"} {
		if c := podStatusColor(s); c != colorRed {
			t.Errorf("podStatusColor(%q) heuristic = %v, want red", s, c)
		}
	}
}

func TestPodStatusColorHeuristicYellow(t *testing.T) {
	for _, s := range []string{"Init:1/3", "PodCreating"} {
		if c := podStatusColor(s); c != colorYellow {
			t.Errorf("podStatusColor(%q) heuristic = %v, want yellow", s, c)
		}
	}
}

func TestPodStatusColorDefault(t *testing.T) {
	if c := podStatusColor("Unknown"); c != colorFg {
		t.Errorf("podStatusColor(\"Unknown\") = %v, want default fg", c)
	}
}

// ---------------------------------------------------------------------------
// scrollWindow
// ---------------------------------------------------------------------------

func TestScrollWindowNoScroll(t *testing.T) {
	s, e := scrollWindow(0, 5, 10)
	if s != 0 || e != 5 {
		t.Errorf("scrollWindow(0,5,10) = (%d,%d), want (0,5)", s, e)
	}
}

func TestScrollWindowMiddle(t *testing.T) {
	s, e := scrollWindow(15, 20, 10)
	if s != 6 || e != 16 {
		t.Errorf("scrollWindow(15,20,10) = (%d,%d), want (6,16)", s, e)
	}
}

func TestScrollWindowEnd(t *testing.T) {
	s, e := scrollWindow(19, 20, 10)
	if s != 10 || e != 20 {
		t.Errorf("scrollWindow(19,20,10) = (%d,%d), want (10,20)", s, e)
	}
}

func TestScrollWindowSmall(t *testing.T) {
	s, e := scrollWindow(2, 3, 10)
	if s != 0 || e != 3 {
		t.Errorf("scrollWindow(2,3,10) = (%d,%d), want (0,3)", s, e)
	}
}

// ---------------------------------------------------------------------------
// parsePods
// ---------------------------------------------------------------------------

func makePodJSON(name, phase string, waitReason, termReason string) []byte {
	type cState struct {
		Waiting    *struct{ Reason string } `json:"waiting,omitempty"`
		Terminated *struct{ Reason string } `json:"terminated,omitempty"`
	}
	type cs struct {
		Ready bool   `json:"ready"`
		State cState `json:"state"`
	}
	st := cs{Ready: phase == "Running"}
	if waitReason != "" {
		st.State.Waiting = &struct{ Reason string }{Reason: waitReason}
	}
	if termReason != "" {
		st.State.Terminated = &struct{ Reason string }{Reason: termReason}
	}

	raw := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"metadata": map[string]interface{}{
					"name":              name,
					"creationTimestamp": "2025-01-01T00:00:00Z",
					"labels":            map[string]string{},
					"annotations":       map[string]string{},
				},
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{"image": "myapp:v1.2.3"},
					},
				},
				"status": map[string]interface{}{
					"phase":             phase,
					"containerStatuses": []cs{st},
				},
			},
		},
	}
	b, _ := json.Marshal(raw)
	return b
}

func TestParsePods(t *testing.T) {
	data := makePodJSON("mypod", "Running", "", "")
	pods, err := parsePods(data)
	if err != nil {
		t.Fatalf("parsePods: %v", err)
	}
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(pods))
	}
	if pods[0].Name != "mypod" {
		t.Errorf("name = %q, want mypod", pods[0].Name)
	}
	if pods[0].Status != "Running" {
		t.Errorf("status = %q, want Running", pods[0].Status)
	}
	if pods[0].Version != "v1.2.3" {
		t.Errorf("version = %q, want v1.2.3", pods[0].Version)
	}
}

func TestParsePodsFailReason(t *testing.T) {
	data := makePodJSON("crashpod", "Running", "CrashLoopBackOff", "")
	pods, err := parsePods(data)
	if err != nil {
		t.Fatalf("parsePods: %v", err)
	}
	if pods[0].Status != "CrashLoopBackOff" {
		t.Errorf("status = %q, want CrashLoopBackOff", pods[0].Status)
	}
}

func TestParsePodsInvalidJSON(t *testing.T) {
	_, err := parsePods([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// parseServices
// ---------------------------------------------------------------------------

func TestParseServices(t *testing.T) {
	raw := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"metadata": map[string]interface{}{
					"name":              "mysvc",
					"creationTimestamp": "2025-01-01T00:00:00Z",
				},
				"spec": map[string]interface{}{
					"type":      "ClusterIP",
					"clusterIP": "10.0.0.1",
					"ports": []map[string]interface{}{
						{"port": 8080, "protocol": "TCP"},
					},
				},
				"status": map[string]interface{}{
					"loadBalancer": map[string]interface{}{
						"ingress": []map[string]interface{}{},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(raw)
	svcs, err := parseServices(b)
	if err != nil {
		t.Fatalf("parseServices: %v", err)
	}
	if len(svcs) != 1 {
		t.Fatalf("expected 1 service, got %d", len(svcs))
	}
	if svcs[0].Name != "mysvc" {
		t.Errorf("name = %q, want mysvc", svcs[0].Name)
	}
	if svcs[0].ClusterIP != "10.0.0.1" {
		t.Errorf("clusterIP = %q, want 10.0.0.1", svcs[0].ClusterIP)
	}
	if svcs[0].Ports != "8080/TCP" {
		t.Errorf("ports = %q, want 8080/TCP", svcs[0].Ports)
	}
}

// ---------------------------------------------------------------------------
// parseDeployments
// ---------------------------------------------------------------------------

func TestParseDeployments(t *testing.T) {
	raw := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"metadata": map[string]interface{}{
					"name":              "mydeploy",
					"creationTimestamp": "2025-01-01T00:00:00Z",
				},
				"status": map[string]interface{}{
					"replicas":          3,
					"readyReplicas":     2,
					"updatedReplicas":   3,
					"availableReplicas": 2,
				},
			},
		},
	}
	b, _ := json.Marshal(raw)
	deps, err := parseDeployments(b)
	if err != nil {
		t.Fatalf("parseDeployments: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	if deps[0].Name != "mydeploy" {
		t.Errorf("name = %q, want mydeploy", deps[0].Name)
	}
	if deps[0].Ready != "2/3" {
		t.Errorf("ready = %q, want 2/3", deps[0].Ready)
	}
}
