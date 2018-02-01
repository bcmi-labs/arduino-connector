//
//  This file is part of arduino-connector
//
//  Copyright (C) 2018  Arduino AG (http://www.arduino.cc/)
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package main

import (
	"fmt"
	"time"
)

type heartbeat struct {
	running bool
	send    func(payload int) error
}

func newHeartbeat(sendFunction func(payload int) error) *heartbeat {
	res := &heartbeat{
		running: true,
		send:    sendFunction,
	}
	go res.run()
	return res
}

func (h *heartbeat) run() {
	id := 0
	for h.running {
		time.Sleep(15 * time.Second)
		err := h.send(id)
		id = (id + 1) % 10000
		if err != nil {
			fmt.Println("Error sending heartbeat:", err)
			h.running = false
			return
		}
	}
}

func (h *heartbeat) stop() {
	h.running = false
}