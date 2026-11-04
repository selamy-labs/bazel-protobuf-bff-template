output "serving" {
  description = "What the plan would serve: the image reference by digest, the port and the origin."
  value = {
    image          = local.image
    revision       = local.serving_revision
    port           = local.inputs.runtime.port
    allowed_origin = local.inputs.runtime.allowed_origin
    max_instances  = local.inputs.bounds.max_instances
  }
}
