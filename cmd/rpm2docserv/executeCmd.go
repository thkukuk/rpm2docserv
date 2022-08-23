// Copyright 2022 Thorsten Kukuk
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"log"
)

func executeCmd(command string, arg ...string) (error) {
	var out bytes.Buffer
	var stderr bytes.Buffer
	var err error

	err = nil

	cmd := exec.Command(command, arg...)
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	log.Printf("Executing %s: %v", cmd.Path, cmd.Args)

	if err = cmd.Run(); err != nil {
		log.Printf("Error invoking " + command + ": " + fmt.Sprint(err) + "\n" + stderr.String())
		return err
	} else {
		log.Printf(out.String() + "\n\nErrors: " + stderr.String())
	}

	return err
}
