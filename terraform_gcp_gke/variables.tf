variable "project_id" {
  description = "project id"
  default = "playground-s-11-e2becf2b"
}

variable "region" {
  description = "region"
  default = "us-east1"
}

variable "gke_num_nodes" {
  default     = 1
  description = "number of gke nodes"
}

variable "gke_username" {
  default     = ""
  description = "gke username"
}

variable "gke_password" {
  default     = ""
  description = "gke password"
}

variable "global_domain" {
  default     = "argocd.philipphomberger.com"
  description = "argocddomain"
}
