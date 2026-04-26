locals {
  config   = yamldecode(file("config.yaml"))
  jp_files = [for file in fileset("./", "_jp-*") : file]
  default_namespace_quotas = {
    "requests.cpu"    = "8"
    "requests.memory" = "16Gi"
    "limits.cpu"      = "8"
    "limits.memory"   = "16Gi"
  }
}

resource "terraform_data" "validation" {
  triggers_replace = sha256(
    jsonencode({
      config = local.config
      files  = local.jp_files
    })
  )

  lifecycle {
    precondition {
      condition     = length(local.jp_files) > 0
      error_message = "Please run `jordplate render` before running `terraform plan`."
    }
  }
}
