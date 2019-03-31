#!/usr/bin/env bash

# Copyright 2017 Google Inc. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -ex

header() {
    cat /go/src/agones.dev/agones/build/boilerplate.go.txt $1 >> /tmp/cpp/$1 && mv /tmp/cpp/$1 .
}

googleapis=/go/src/agones.dev/agones/vendor/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis

cd /go/src/agones.dev/agones/sdks/cpp
find -name '*.pb.*' -delete

cd /go/src/agones.dev/agones
protoc -I ${googleapis} -I . --grpc_out=./sdks/cpp --plugin=protoc-gen-grpc=`which grpc_cpp_plugin` sdk.proto
protoc -I ${googleapis} -I . --cpp_out=./sdks/cpp sdk.proto ${googleapis}/google/api/annotations.proto  ${googleapis}/google/api/http.proto

mkdir -p /tmp/cpp

cd ./sdks/cpp
header sdk.pb.h
header sdk.grpc.pb.cc
header sdk.pb.cc

cd ./google/api/
header annotations.pb.cc
header annotations.pb.h
header http.pb.cc
header http.pb.h

rm -r /tmp/cpp
