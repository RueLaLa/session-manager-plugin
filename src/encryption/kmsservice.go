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

package encryption

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/session-manager-plugin/src/sdkutil"
)

// KMSKeySizeInBytes is the key size that is fetched from KMS. 64 bytes key is split into two halves.
// First half 32 bytes key is used by agent for encryption and second half 32 bytes by clients like cli/console
const KMSKeySizeInBytes int32 = 64

func NewKMSService() (kmsService *kms.Client, err error) {
	kmsService = kms.NewFromConfig(sdkutil.GetSDKConfig())
	return kmsService, nil
}

// GenerateDataKey gets cipher text and plain text keys from KMS service
func KMSGenerateDataKey(kmsKeyId string, ctx map[string]string) (cipherTextKey []byte, plainTextKey []byte, err error) {
	svc, _ := NewKMSService()
	kmsKeySize := KMSKeySizeInBytes
	generateDataKeyInput := kms.GenerateDataKeyInput{
		KeyId:             &kmsKeyId,
		NumberOfBytes:     &kmsKeySize,
		EncryptionContext: ctx,
	}

	var generateDataKeyOutput *kms.GenerateDataKeyOutput
	if generateDataKeyOutput, err = svc.GenerateDataKey(context.TODO(), &generateDataKeyInput); err != nil {
		return nil, nil, fmt.Errorf("error calling KMS GenerateDataKey API: %s", err)
	}

	return generateDataKeyOutput.CiphertextBlob, generateDataKeyOutput.Plaintext, nil
}
