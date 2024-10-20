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

// Package sdkutil provides utilities used to call awssdk.
package sdkutil

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

var defaultProfile string

func GetSDKConfig() aws.Config {
	scp, _ := config.LoadSharedConfigProfile(context.TODO(), defaultProfile)
	env_region, env_present := os.LookupEnv("AWS_REGION")

	if env_present {
		scp.Region = env_region
	} else if scp.Region == "" {
		scp.Region = "us-east-1"
	}

	cfg, _ := config.LoadDefaultConfig(
		context.TODO(),
		config.WithSharedConfigProfile(defaultProfile),
		config.WithDefaultRegion(scp.Region),
	)

	return cfg
}

func SetProfile(profile string) {
	defaultProfile = profile
}
