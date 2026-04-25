locals {
  app_name    = "myservice"
  environment = env("DEPLOY_ENV", "dev")

  # exec() runs an external command and captures stdout (requires --allow-exec).
  build_id = exec("git", "rev-parse", "--short", "HEAD")

  image = "registry.example.com/${local.app_name}:${local.build_id}"

  tags = {
    app = local.app_name
    env = local.environment
  }

  tag_string = join(",", [for key in try(keys(local.tags), []) : key])
}

template "deployment" {
  for_each = {
    ha = {
      filename    = "ha-deployment"
      replicas    = 3
      environment = "prod"
    }
    single = {
      filename = "single-deployment"
      replicas = 1
    }
    disabled = {
      filename = "disabled-deployment"
    }
  }
  source      = "templates/deployment.yaml.j2"
  destination = "out/${local.environment}/${each.value.filename}.yaml"
  disabled    = contains(["disabled", ], each.key)
  values = {
    app_name    = local.app_name
    image       = local.image
    environment = try(each.value.environment, local.environment)
    replicas    = try(each.value.replicas, 6)
    tags        = merge(local.tags, { deployment_key = each.key })
    tag_string  = local.tag_string
  }
}
