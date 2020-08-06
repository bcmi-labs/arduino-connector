// +build functional

package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	docker "github.com/docker/docker/client"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

// NewMqttTestClientLocal creates mqtt client in localhost:1883
func NewMqttTestClientLocal() *MqttTestClient {
	uiOptions := mqtt.NewClientOptions().AddBroker("tcp://localhost:1883").SetClientID("UI")
	ui := mqtt.NewClient(uiOptions)
	if token := ui.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	return &MqttTestClient{
		ui,
		"",
	}
}

type testStatus struct {
	appStatus *Status
	ui        *MqttTestClient
}

var ts testStatus

func TestMain(m *testing.M) {
	os.Exit(setupAndRun(m))
}

func setupAndRun(m *testing.M) int {
	ts.ui = NewMqttTestClientLocal()
	defer ts.ui.Close()

	ts.appStatus = NewStatus(program{}.Config, nil, nil, "")
	ts.appStatus.dockerClient, _ = docker.NewClientWithOpts(docker.WithVersion("1.38"))
	ts.appStatus.mqttClient = mqtt.NewClient(mqtt.NewClientOptions().AddBroker("tcp://localhost:1883").SetClientID("arduino-connector"))

	if token := ts.appStatus.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	}
	defer ts.appStatus.mqttClient.Disconnect(100)

	return m.Run()
}

func TestDockerPsApi(t *testing.T) {
	subscribeTopic(ts.appStatus.mqttClient, "0", "/containers/ps/post", ts.appStatus, ts.appStatus.ContainersPsEvent, false)
	resp := ts.ui.MqttSendAndReceiveTimeout(t, "/containers/ps", "{}", 50*time.Millisecond)

	// ask Docker about containers effectively running
	cmd := exec.Command("bash", "-c", "docker ps -a")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(string(out), "\n")
	// Remove the first line (command output header) and the last line (empty line)
	lines = lines[1 : len(lines)-1]

	// Take json without INFO tag
	resp = strings.TrimPrefix(resp, "INFO: ")
	resp = strings.TrimSuffix(resp, "\n\n")
	var result []types.Container
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, len(result), len(lines))
	for i, line := range lines {
		containerId := strings.Fields(line)[0]
		assert.True(t, strings.HasPrefix(result[i].ID, containerId))
	}
}

func TestDockerListImagesApi(t *testing.T) {
	subscribeTopic(ts.appStatus.mqttClient, "0", "/containers/images/post", ts.appStatus, ts.appStatus.ContainersListImagesEvent, false)
	resp := ts.ui.MqttSendAndReceiveTimeout(t, "/containers/images", "{}", 50*time.Millisecond)

	// ask Docker about images effectively present
	cmd := exec.Command("bash", "-c", "docker images -a")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(string(out), "\n")
	// Remove the first line (command output header) and the last line (empty line)
	lines = lines[1 : len(lines)-1]

	// Take json without INFO tag
	resp = strings.TrimPrefix(resp, "INFO: ")
	resp = strings.TrimSuffix(resp, "\n\n")
	var result []types.ImageSummary
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, len(result), len(lines))
}

func TestDockerRenameApi(t *testing.T) {
	// download an alpine image from library to use as test
	reader, err := ts.appStatus.dockerClient.ImagePull(context.Background(), "docker.io/library/alpine", types.ImagePullOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(ioutil.Discard, reader)
	if err != nil {
		t.Error(err)
	}

	defer func() {
		reader.Close()

		filters := filters.NewArgs(filters.Arg("reference", "alpine"))
		images, errImagels := ts.appStatus.dockerClient.ImageList(context.Background(), types.ImageListOptions{Filters: filters})
		if errImagels != nil {
			t.Fatal(errImagels)
		}

		if _, errImageRemove := ts.appStatus.dockerClient.ImageRemove(context.Background(), images[0].ID, types.ImageRemoveOptions{}); errImageRemove != nil {
			t.Fatal(errImageRemove)
		}
	}()

	// create a test container from downloaded image
	createContResp, err := ts.appStatus.dockerClient.ContainerCreate(context.Background(), &container.Config{
		Image: "alpine",
		Cmd:   []string{"echo", "hello world"},
	}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = ts.appStatus.dockerClient.ContainerRemove(context.Background(), createContResp.ID, types.ContainerRemoveOptions{}); err != nil {
			t.Fatal(err)
		}
	}()

	cnPayload := ChangeNamePayload{
		ContainerID:   createContResp.ID,
		ContainerName: "newname",
	}
	data, err := json.Marshal(cnPayload)
	if err != nil {
		t.Fatal(err)
	}

	subscribeTopic(ts.appStatus.mqttClient, "0", "/containers/rename/post", ts.appStatus, ts.appStatus.ContainersRenameEvent, true)
	resp := ts.ui.MqttSendAndReceiveTimeout(t, "/containers/rename", string(data), 250*time.Millisecond)

	// ask Docker about containers
	cmd := exec.Command("bash", "-c", "docker container ls -a")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(string(out), "\n")
	// Remove the first line (command output header) and the last line (empty line)
	lines = lines[1 : len(lines)-1]

	// Take json without INFO tag
	resp = strings.TrimPrefix(resp, "INFO: ")
	resp = strings.TrimSuffix(resp, "\n\n")
	var result ChangeNamePayload
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, cnPayload, result)

	// find test container through its ID and check its name
	for _, line := range lines {
		tokens := strings.Fields(line)
		if strings.HasPrefix(result.ContainerID, tokens[0]) {
			assert.Equal(t, result.ContainerName, tokens[len(tokens)-1])
			return
		}
	}

	t.Fatalf("no container with ID %s has been found\n", result.ContainerID)
}

func TestDockerActionRunApi(t *testing.T) {
	subscribeTopic(ts.appStatus.mqttClient, "0", "/containers/action/post", ts.appStatus, ts.appStatus.ContainersActionEvent, false)
	payload := map[string]interface{}{"action": "run", "image": "alpine", "name": "test-container"}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Error(err)
	}

	ts.ui.MqttSendAndReceiveTimeout(t, "/containers/action", string(data), 10*time.Second)

	// Check real container runnig with bash command
	cmd := exec.Command("bash", "-c", "docker ps")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(string(out), "\n")
	// Remove the first line (command output header) and the last line (empty line)
	lines = lines[1 : len(lines)-1]

	assert.Equal(t, len(lines), 1)
	assert.True(t, strings.Contains(lines[0], "alpine"))
	assert.True(t, strings.Contains(lines[0], "test-container"))

	// Clean up
	timeout := 1 * time.Millisecond
	err = ts.appStatus.dockerClient.ContainerStop(context.Background(), "test-container", &timeout)
	if err != nil {
		t.Error(err)
	}

	err = ts.appStatus.dockerClient.ContainerRemove(context.Background(), "test-container", types.ContainerRemoveOptions{})
	if err != nil {
		t.Error(err)
	}

	_, err = ts.appStatus.dockerClient.ImageRemove(context.Background(), "alpine", types.ImageRemoveOptions{})
	if err != nil {
		t.Error(err)
	}
}

type MqttTokenMock struct {
	returnErr bool
}

func (t *MqttTokenMock) Wait() bool {
	return true
}

func (t *MqttTokenMock) WaitTimeout(time.Duration) bool {
	return true
}

func (t *MqttTokenMock) Error() error {
	if t.returnErr {
		return errors.New("test err")
	}

	return nil
}

type MqttClientMock struct {
	errPublished string
}

func (c *MqttClientMock) IsConnected() bool {
	return false
}

func (c *MqttClientMock) IsConnectionOpen() bool {
	return true
}

func (c *MqttClientMock) Connect() mqtt.Token {
	return nil
}

func (c *MqttClientMock) Disconnect(quiesce uint) {
}

func (c *MqttClientMock) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	payloadStr := payload.(string)
	if strings.HasPrefix(payloadStr, "INFO") {
		return &MqttTokenMock{returnErr: true}
	}

	c.errPublished = payloadStr
	return &MqttTokenMock{returnErr: false}
}

func (c *MqttClientMock) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token {
	return nil
}

func (c *MqttClientMock) SubscribeMultiple(filters map[string]byte, callback mqtt.MessageHandler) mqtt.Token {
	return nil
}

func (c *MqttClientMock) Unsubscribe(topics ...string) mqtt.Token {
	return nil
}

func (c *MqttClientMock) AddRoute(topic string, callback mqtt.MessageHandler) {
}

func (c *MqttClientMock) OptionsReader() mqtt.ClientOptionsReader {
	return mqtt.ClientOptionsReader{}
}

func TestDockerApiError(t *testing.T) {
	ts.appStatus.mqttClient = &MqttClientMock{}
	topic := "/container/ps"
	ts.appStatus.SendInfo(topic, "error")
	mockClient := ts.appStatus.mqttClient.(*MqttClientMock)
	assert.Equal(t, mockClient.errPublished, "ERROR: test err\n")
}
