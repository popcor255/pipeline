/*
Copyright 2020 The Tekton Authors

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

package test

import (
	"archive/tar"
	"io"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	tkremote "github.com/tektoncd/pipeline/pkg/remote/oci"
)

func TestCreateImage(t *testing.T) {
	// Set up a fake registry to push an image to.
	s := httptest.NewServer(registry.New())
	defer s.Close()
	u, _ := url.Parse(s.URL)

	task := &v1beta1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-create-image",
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1beta1",
			Kind:       "Task",
		},
	}

	ref, err := CreateImage(u.Host+"/task/test-create-image", task)
	if err != nil {
		t.Errorf("uploading image failed unexpectedly with an error: %w", err)
	}

	// Pull the image and ensure the layers are composed correctly.
	imgRef, err := name.ParseReference(ref)
	if err != nil {
		t.Errorf("digest %s is not a valid reference: %w", ref, err)
	}

	img, err := remote.Image(imgRef, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		t.Errorf("could not fetch created image: %w", err)
	}

	m, err := img.Manifest()
	if err != nil {
		t.Errorf("failed to fetch img manifest: %w", err)
	}

	layers, err := img.Layers()
	if err != nil || len(layers) != 1 {
		t.Errorf("img layers were incorrect. Num Layers: %d. Err: %w", len(layers), err)
	}

	if diff := cmp.Diff(m.Layers[0].Annotations[tkremote.TitleAnnotation], "test-create-image"); diff != "" {
		t.Error(diff)
	}
	if diff := cmp.Diff(m.Layers[0].Annotations[tkremote.KindAnnotation], "task"); diff != "" {
		t.Error(diff)
	}
	if diff := cmp.Diff(m.Layers[0].Annotations[tkremote.APIVersionAnnotation], "v1beta1"); diff != "" {
		t.Error(diff)
	}

	// Read the layer's contents and ensure it matches the task we uploaded.
	rc, err := layers[0].Uncompressed()
	if err != nil {
		t.Errorf("layer contents were corrupted: %w", err)
	}
	defer rc.Close()

	reader := tar.NewReader(rc)
	header, err := reader.Next()
	if err != nil {
		t.Errorf("failed to load tar bundle: %w", err)
	}

	actual := make([]byte, header.Size)
	if _, err := reader.Read(actual); err != nil && err != io.EOF {
		t.Errorf("failed to read contents of tar bundle: %w", err)
	}

	raw, err := yaml.Marshal(task)
	if err != nil {
		t.Fatalf("Could not marshal task to bytes: %#v", err)
	}
	if diff := cmp.Diff(string(raw), string(actual)); diff != "" {
		t.Error(diff)
	}
}
