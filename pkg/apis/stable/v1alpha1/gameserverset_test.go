// Copyright 2018 Google Inc. All Rights Reserved.
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

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGameServerSetGameServer(t *testing.T) {
	gss := GameServerSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "namespace",
			UID:       "1234",
		},
		Spec: GameServerSetSpec{
			Replicas: 10,
			Template: GameServerTemplateSpec{
				Spec: GameServerSpec{
					ContainerPort: 1234,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "container", Image: "myimage"}},
						},
					},
				},
			},
		},
	}

	gs := gss.GameServer()
	assert.Equal(t, "", gs.ObjectMeta.Name)
	assert.Equal(t, gss.ObjectMeta.Namespace, gs.ObjectMeta.Namespace)
	assert.Equal(t, gss.ObjectMeta.Name+"-", gs.ObjectMeta.GenerateName)
	assert.Equal(t, gss.ObjectMeta.Name, gs.ObjectMeta.Labels[GameServerSetGameServerLabel])
	assert.Equal(t, gs.Spec, gss.Spec.Template.Spec)
	assert.True(t, v1.IsControlledBy(gs, &gss))
}
