// Copyright © 2019 Banzai Cloud
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

package patch

import (
	"fmt"
	"reflect"

	"emperror.dev/errors"
	"github.com/disaster37/k8s-objectmatcher/patch"
	json "github.com/json-iterator/go"
)

type CalculateOption func([]byte, []byte) ([]byte, []byte, error)

var DefaultPatchMaker = NewPatchMaker(&patch.K8sStrategicMergePatcher{}, &patch.BaseJSONMergePatcher{})

type Maker interface {
	Calculate(currentObject, modifiedObject, originalObject any, opts ...CalculateOption) (*PatchResult, error)
}

type PatchMaker struct {
	strategicMergePatcher patch.StrategicMergePatcher
	jsonMergePatcher      patch.JSONMergePatcher
}

func NewPatchMaker(strategicMergePatcher patch.StrategicMergePatcher, jsonMergePatcher patch.JSONMergePatcher) Maker {
	return &PatchMaker{
		strategicMergePatcher: strategicMergePatcher,
		jsonMergePatcher:      jsonMergePatcher,
	}
}

func (p *PatchMaker) Calculate(currentObject, modifiedObject, originalObject any, opts ...CalculateOption) (*PatchResult, error) {

	current, err := json.ConfigCompatibleWithStandardLibrary.Marshal(currentObject)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to convert current object to byte sequence")
	}
	currentOrg := make([]byte, len(current))
	copy(currentOrg, current)

	modified, err := json.ConfigCompatibleWithStandardLibrary.Marshal(modifiedObject)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to convert current object to byte sequence")
	}

	for _, opt := range opts {
		current, modified, err = opt(current, modified)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to apply option function")
		}
	}

	original, err := json.ConfigCompatibleWithStandardLibrary.Marshal(originalObject)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to convert current object to byte sequence")
	}

	var patch []byte
	var patchCurrent []byte
	var patched any

	patch, patchCurrent, err = p.jsonMergePatch(original, modified, current, currentOrg)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate merge patch")
	}

	patched = reflect.New(reflect.ValueOf(currentObject).Elem().Type()).Interface()
	if err = json.Unmarshal(patchCurrent, patched); err != nil {
		return nil, errors.Wrap(err, "Failed to create patched object")
	}

	return &PatchResult{
		Patch:    patch,
		Current:  current,
		Modified: modified,
		Original: original,
		Patched:  patched,
	}, nil

}

func (p *PatchMaker) jsonMergePatch(original, modified, current, currentOrg []byte) ([]byte, []byte, error) {

	patch, err := p.jsonMergePatcher.CreateThreeWayJSONMergePatch(original, modified, current)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to generate merge patch")
	}

	var patchedCurrent []byte

	// Apply the patch to the current object and create a merge patch to see if there has any effective changes been made
	if string(patch) != "{}" {
		// apply the patch
		patchCurrent, err := p.jsonMergePatcher.MergePatch(current, patch)
		if err != nil {
			return nil, nil, errors.Wrap(err, "Failed to merge generated patch to current object")
		}
		// create the patch again, but now between the current and the patched version of the current object
		patch, err = p.jsonMergePatcher.CreateMergePatch(current, patchCurrent)
		if err != nil {
			return nil, nil, errors.Wrap(err, "Failed to create patch between the current and patched current object")
		}

		patchedCurrent, err = p.jsonMergePatcher.MergePatch(currentOrg, patch)
		if err != nil {
			return nil, nil, errors.Wrap(err, "Failed to apply patch")
		}
	} else {
		patchedCurrent = currentOrg
	}
	return patch, patchedCurrent, err
}

type PatchResult struct {
	Patch    []byte
	Current  []byte
	Modified []byte
	Original []byte
	Patched  any
}

func (p *PatchResult) IsEmpty() bool {
	return string(p.Patch) == "{}"
}

func (p *PatchResult) String() string {
	return fmt.Sprintf("\nPatch: %s \nCurrent: %s\nModified: %s\nOriginal: %s\n", p.Patch, p.Current, p.Modified, p.Original)
}