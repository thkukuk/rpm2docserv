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
	"time"
)


func zypperDownload(packages []string, start time.Time) (error) {

	args := []string{"--pkg-cache-dir",
		"/var/cache/rpm2docserv/repo",
		"--disable-system-resolvables",
		"--non-interactive",
		"install",
		"--auto-agree-with-licenses",
		"--auto-agree-with-product-licenses",
		"--download-only"}

	args = append (args, packages...)

	err := executeCmd("zypper", args...)

	return err
}
