// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may not
// use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
// either express or implied. See the License for the specific language governing
// permissions and limitations under the License.

// Package jsonutil contains various utilities for dealing with json data.
package jsonutil

import (
	"encoding/json"
)

func Remarshal(obj interface{}, remarshalledObj interface{}) (err error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return
	}
	err = json.Unmarshal(b, remarshalledObj)
	if err != nil {
		return
	}
	return nil
}
