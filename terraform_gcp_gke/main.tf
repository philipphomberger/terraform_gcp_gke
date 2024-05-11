terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "5.28.0"
    }
  }

  required_version = ">= 0.14"
}

provider "google" {
  project = var.project_id
  region  = var.region
}

resource "google_compute_network" "vpc" {
  name                    = "${var.project_id}-vpc"
  auto_create_subnetworks = "false"
}


resource "google_compute_subnetwork" "subnet" {
  name          = "${var.project_id}-subnet"
  region        = var.region
  network       = google_compute_network.vpc.name
  ip_cidr_range = "10.10.0.0/24"
}


data "google_container_engine_versions" "gke_version" {
  location = var.region
  version_prefix = "1.28."
}


resource "google_container_cluster" "primary" {
  name     = "${var.project_id}-gke"
  location = var.region

  remove_default_node_pool = true
  initial_node_count       = 1

  node_config {
    machine_type = "n1-standard-1"
    disk_size_gb = 50
  }

  network    = google_compute_network.vpc.name
  subnetwork = google_compute_subnetwork.subnet.name
  

}

resource "google_container_node_pool" "primary_nodes" {
  name       = google_container_cluster.primary.name
  location   = var.region
  cluster    = google_container_cluster.primary.name
  
  version = data.google_container_engine_versions.gke_version.release_channel_latest_version["STABLE"]
  node_count = var.gke_num_nodes

  node_config {
    oauth_scopes = [
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
    ]

    labels = {
      env = var.project_id
    }

    # preemptible  = true
    machine_type = "n1-standard-1"
    disk_size_gb = 50
    tags         = ["gke-node", "${var.project_id}-gke"]
    metadata = {
      disable-legacy-endpoints = "true"
    }
  }
}

provider "kubernetes" {
  host     = "https://${google_container_cluster.primary.endpoint}"
  token                  = "${data.google_client_config.current.access_token}"
  client_certificate     = "${base64decode(google_container_cluster.primary.master_auth.0.client_certificate)}"
  client_key             = "${base64decode(google_container_cluster.primary.master_auth.0.client_key)}"
  cluster_ca_certificate = "${base64decode(google_container_cluster.primary.master_auth.0.cluster_ca_certificate)}"
}

data "google_client_config" "current" {}

provider "helm" {
  kubernetes {
    host                   = "https://${google_container_cluster.primary.endpoint}"
    token                  = "${data.google_client_config.current.access_token}"
    client_certificate     = "${base64decode(google_container_cluster.primary.master_auth.0.client_certificate)}"
    client_key             = "${base64decode(google_container_cluster.primary.master_auth.0.client_key)}"
    cluster_ca_certificate = "${base64decode(google_container_cluster.primary.master_auth.0.cluster_ca_certificate)}"
  }
}

resource "helm_release" "nginx_ingress" {
  name       = "ingress-nginx"
  repository = "https://charts.bitnami.com/bitnami"
  chart      = "nginx-ingress-controller"
  namespace  = "default"
  depends_on = [
    google_container_cluster.primary,
    google_container_node_pool.primary_nodes
  ]
}

data "kubernetes_service" "nginx_ingress" {
  metadata {
    name = "ingress-nginx-nginx-ingress-controller"
  }
  depends_on = [
    google_container_cluster.primary,
    google_container_node_pool.primary_nodes,
    helm_release.nginx_ingress
  ]
}

resource "helm_release" "argo_cd" {
  name             = "argocd"
  repository       = "https://argoproj.github.io/argo-helm"
  chart            = "argo-cd"
  namespace        = "argocd"
  version          = "6.7.18"
  create_namespace = true

  set {
    name  = "global.domain"
    value = var.global_domain
  }

  values = [
    "${file("values-argus.yaml")}"
  ]

  depends_on = [
    google_container_cluster.primary,
    google_container_node_pool.primary_nodes,
    helm_release.nginx_ingress
  ]
}
