// Copyright 2016 CoreOS, Inc.
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

package gcloud

import (
	"fmt"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/api/compute/v1"
)

type ImageSpec struct {
	SourceImage           string
	Family                string
	Name                  string
	Description           string
	Licenses              []string // short names
	DisableSCSIMultiqueue bool     // TODO(bgilbert): Remove after stable > 1409.0.0
}

// CreateImage creates an image on GCE and returns operation details and
// a Pending. If overwrite is true, an existing image will be overwritten
// if it exists.
func (a *API) CreateImage(spec *ImageSpec, overwrite bool) (*compute.Operation, *Pending, error) {
	licenses := make([]string, len(spec.Licenses))
	for i, l := range spec.Licenses {
		license, err := a.compute.Licenses.Get(a.options.Project, l).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("Invalid GCE license %s: %v", l, err)
		}
		licenses[i] = license.SelfLink
	}

	if overwrite {
		plog.Debugf("Overwriting image %q", spec.Name)
		// delete existing image, ignore error since it might not exist.
		op, err := a.compute.Images.Delete(a.options.Project, spec.Name).Do()

		if op != nil {
			doable := a.compute.GlobalOperations.Get(a.options.Project, op.Name)
			if err := a.NewPending(op.Name, doable).Wait(); err != nil {
				return nil, nil, err
			}
		}

		// don't return error when delete fails because image doesn't exist
		if err != nil && !strings.HasSuffix(err.Error(), "notFound") {
			return nil, nil, fmt.Errorf("deleting image: %v", err)
		}
	}

	features := []*compute.GuestOsFeature{
		&compute.GuestOsFeature{
			Type: "VIRTIO_SCSI_MULTIQUEUE",
		},
	}
	if spec.DisableSCSIMultiqueue {
		features = []*compute.GuestOsFeature{}
	}
	image := &compute.Image{
		Family:          spec.Family,
		Name:            spec.Name,
		Description:     spec.Description,
		Licenses:        licenses,
		GuestOsFeatures: features,
		RawDisk: &compute.ImageRawDisk{
			Source: spec.SourceImage,
		},
	}

	plog.Debugf("Creating image %q from %q", spec.Name, spec.SourceImage)

	op, err := a.compute.Images.Insert(a.options.Project, image).Do()
	if err != nil {
		return nil, nil, err
	}

	doable := a.compute.GlobalOperations.Get(a.options.Project, op.Name)
	return op, a.NewPending(op.Name, doable), nil
}

func (a *API) ListImages(ctx context.Context, prefix string) ([]*compute.Image, error) {
	var images []*compute.Image
	listReq := a.compute.Images.List(a.options.Project)
	if prefix != "" {
		listReq.Filter(fmt.Sprintf("name eq ^%s.*", prefix))
	}
	err := listReq.Pages(ctx, func(i *compute.ImageList) error {
		images = append(images, i.Items...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("Listing GCE images failed: %v", err)
	}
	return images, nil
}
