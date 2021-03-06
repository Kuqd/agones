// Copyright 2019 Google LLC All Rights Reserved.
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


// Run:
//  terraform apply -var project="<YOUR_GCP_ProjectID>" [-var agones_version="1.4.0"]

provider "google" {
  version = "~> 2.10"
}

provider "google-beta" {
  version = "~> 2.10"
}

variable "project" {
  default = ""
}

variable "name" {
  default = "agones-terraform-example"
}

// Install latest version of agones
variable "agones_version" {
  default = ""
}

variable "machine_type" {
  default = "n1-standard-4"
}

// Note: This is the number of gameserver nodes. The Agones module will automatically create an additional
// two node pools with 1 node each for "agones-system" and "agones-metrics".
variable "node_count" {
  default = "4"
}

variable "zone" {
  default     = "us-west1-c"
  description = "The GCP zone to create the cluster in"
}

variable "network" {
  default     = "default"
  description = "The name of the VPC network to attach the cluster and firewall rule to"
}

variable "log_level" {
  default = "info"
}

variable "feature_gates" {
  default = ""
}

module "gke_cluster" {
  // ***************************************************************************************************
  // Update ?ref= to the agones release you are installing. For example, ?ref=release-1.3.0 corresponds
  // to Agones version 1.3.0
  // ***************************************************************************************************
  source = "git::https://github.com/googleforgames/agones.git//install/terraform/modules/gke/?ref=master"

  cluster = {
    "name"             = var.name
    "zone"             = var.zone
    "machineType"      = var.machine_type
    "initialNodeCount" = var.node_count
    "project"          = var.project
    "network"          = var.network
  }
}

module "helm_agones" {
  // ***************************************************************************************************
  // Update ?ref= to the agones release you are installing. For example, ?ref=release-1.3.0 corresponds
  // to Agones version 1.3.0
  // ***************************************************************************************************
  source = "git::https://github.com/googleforgames/agones.git//install/terraform/modules/helm/?ref=master"

  agones_version         = var.agones_version
  values_file            = ""
  chart                  = "agones"
  feature_gates          = var.feature_gates
  host                   = module.gke_cluster.host
  token                  = module.gke_cluster.token
  cluster_ca_certificate = module.gke_cluster.cluster_ca_certificate
  log_level              = var.log_level
}

output "host" {
  value = module.gke_cluster.host
}
output "token" {
  value = module.gke_cluster.token
}
output "cluster_ca_certificate" {
  value = module.gke_cluster.cluster_ca_certificate
}
