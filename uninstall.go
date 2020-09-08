package main

import (
	"os"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Uninstall remove all service installed and create script to
// removing application from machine
func (s *Status) Uninstall(client mqtt.Client, msg mqtt.Message) {
	data := "OK"

	removeSketches(s)

	s.SendInfo(s.topicPertinence+"/status/uninstall", string(data))
}

func removeSketches(s *Status) error {
	folder, err := getSketchFolder(s)
	if err != nil {
		return err
	}

	err = os.RemoveAll(folder)
	if err != nil {
		return err
	}

	return nil
}
