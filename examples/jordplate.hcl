locals {
  app_name    = "myservice"
  environment = env("DEPLOY_ENV", "dev")

  # exec() runs an external command and captures stdout (requires --allow-exec).
  # In a real repo you might use: exec("git", "rev-parse", "--short", "HEAD")
  build_id = exec("date", "+%Y%m%d-%H%M%S")

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
      filename = "ha-deployment"
      replicas = 3
    }
    single = {
      filename = "single-deployment"
      replicas = 1
    }
  }
  source      = "templates/deployment.yaml.j2"
  destination = "out/${local.environment}/${each.value.filename}.yaml"
  values = {
    app_name    = local.app_name
    image       = local.image
    environment = local.environment
    replicas    = each.value.replicas
    tags        = merge(local.tags, { deployment_key = each.key })
    tag_string  = local.tag_string
  }
}
