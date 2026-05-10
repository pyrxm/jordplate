terraform {
  required_version = "> 1"
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 3"
    }
    kubectl = {
      source  = "alekc/kubectl"
      version = "~> 2"
    }
  }
}
