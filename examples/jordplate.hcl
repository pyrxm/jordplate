pre_hook "echo" {
  command = ["echo", "hello world"]
}

post_hook "list_files" {
  command = ["ls", "-l", "out/"]
}

post_hook "size_files" {
  depends_on = ["list_files"]
  command    = ["du", "-sh", "out/"]
}

post_hook "archive_files" {
  depends_on = ["size_files"]
  command    = ["tar", "cvzf", format("%s.tar.gz", local.archive_file), "out/"]
}

locals {
  config          = yamldecode(file("config.yaml"))
  template_dir    = "jordplate_templates.d/"
  template_prefix = "out/_jp"
  archive_file    = "_jp-rendered"

  shell_base = {
    name   = "Nobody"
    values = ["hello"]
    map = {
      alpha = {
        this = true
      }
      beta = {
        hello = {
          world = "true"
        }
      }
    }
  }

  kubernetes_fqdn = "kubernetes.mynetwork.com"

  kubernetes_clusters = {
    for k, v in try(local.config["clusters"], {}) : k => {
      environment = startswith(k, "prod-") ? "prod" : "nonprod"
      namespaces  = keys(v.namespaces)
      enabled     = try(v.enabled, true)
    }
  }

  deployments = {
    # Iterate over each cluster+namespace map
    for item in flatten([
      for cluster, cluster_conf in local.kubernetes_clusters : [
        for namespace in try(cluster_conf.namespaces, []) : {
          key         = join("-", [cluster, namespace])
          platform    = get_platform()
          alias       = cluster
          environment = cluster_conf.environment
          namespace   = join("-", [local.config.application_name, namespace])
          filename    = join("-", [local.template_prefix, cluster, cluster_conf.environment, namespace])
          app_name    = local.config.application_name
          image       = join(":", [try(local.config.image, local.config.application_name), try(local.config.version, "latest")])
        }
      ] if cluster_conf.enabled
      ]) : item.key => merge(
      # Take original configuration
      item,
      # Merge in deployment configuration
      { for k, v in try(local.config.deployments[item.environment], {}) : k => v }
    )
  }
}

template "kubernetes_provider" {
  for_each    = local.kubernetes_clusters
  source      = format("%s/provider.tf.j2", local.template_dir)
  destination = format("%s-%s-provider.tf", local.template_prefix, each.key)
  values = {
    hostname = join(".", [each.key, each.value.environment, local.kubernetes_fqdn])
    alias    = each.key
  }
}

template "kubernetes_namespace" {
  for_each    = local.kubernetes_clusters
  source      = format("%s/namespaces.tf.j2", local.template_dir)
  destination = format("%s-%s-namespaces.tf", local.template_prefix, each.key)
  enabled     = each.value.enabled
  values = {
    alias = each.key
  }
}

template "kubernetes_deployment_manifests" {
  for_each    = local.deployments
  source      = format("%s/deployment.yaml.j2", local.template_dir)
  destination = format("%s.yaml", each.value.filename)
  values      = each.value
}

template "kubernetes_deployment" {
  for_each    = local.deployments
  source      = format("%s/deployment.tf.j2", local.template_dir)
  destination = format("%s.tf", each.value.filename)
  values      = each.value
}

template "shell_script" {
  for_each = {
    nonprod = {
      name   = "Non-Production"
      values = ["I", "am", "nonprod..."]
      map = {
        alpha = { that = true }
        beta  = { hello = { there = true } }
      }
    }
    prod = {
      name   = "Production"
      values = ["WARNING!!!", "This", "is", "production!"]
      map = {
        alpha = { this = false }
      }
    }
  }

  source      = format("%s/deep_merge.sh.j2", local.template_dir)
  destination = format("%s-%s-deep_merge.sh", local.template_prefix, each.key)
  values = {
    name          = each.value.name
    merged_values = deep_merge(local.shell_base, each.value, { append_slices = true, merge_slice_items = true }).values
    merged_json   = jsonencode(deep_merge(local.shell_base, each.value, { append_slices = true, merge_slice_items = true }))
  }
}
