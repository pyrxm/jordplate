locals {
  config          = yamldecode(file("config.yaml"))
  template_dir    = "jordplate_templates.d/"
  template_prefix = "_jp"

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
