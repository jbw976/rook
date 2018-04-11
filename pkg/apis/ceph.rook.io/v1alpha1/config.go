/*
Copyright 2018 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package v1alpha1

import "strconv"

const (
	StoreTypeKey      = "storeType"
	WalSizeMBKey      = "walSizeMB"
	DatabaseSizeMBKey = "databaseSizeMB"
	JournalSizeMBKey  = "journalSizeMB"
)

func ResolveConfig(nodeConfig map[string]string) StoreConfig {
	storeConfig := StoreConfig{}
	for k, v := range nodeConfig {
		switch k {
		case StoreTypeKey:
			storeConfig.StoreType = v
		case WalSizeMBKey:
			storeConfig.WalSizeMB = convertToIntIgnoreErr(v)
		case DatabaseSizeMBKey:
			storeConfig.DatabaseSizeMB = convertToIntIgnoreErr(v)
		case JournalSizeMBKey:
			storeConfig.JournalSizeMB = convertToIntIgnoreErr(v)
		}
	}

	return storeConfig
}

func convertToIntIgnoreErr(raw string) int {
	val, err := strconv.Atoi(raw)
	if err != nil {
		val = 0
	}

	return val
}
