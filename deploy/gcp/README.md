# Hosted example on GCP

The template's own hosted example (spec 012): the synthetic application deployed by Cloud Build into a dedicated,
BFF-owned project from the exact image spec 009 builds, by digest, with the static frontend on GitHub Pages.
Nothing here is applied by this repository; every create, apply, access, billing and spend step is a recorded
operator gate, and no resource exists yet. The reusable core stays portable: none of this is required to use the
template.

| Path | What it holds |
| --- | --- |
| `inputs/` | the one typed funnel: `inputs.schema.json`, a synthetic `example.inputs.json`, and the validator every plan runs first. Real inputs, state, plans and evidence stay outside the repository |
| `service/`, `cloudbuild/`, `tests/` | the service module, the CD pipeline and the offline tests, as spec 012 T012 and T013 land |

The infrastructure owner composes the project, bootstrap, billing, APIs, IAM and state in its own repository and
publishes a versioned output contract; `inputs.schema.json` names the fields this repository consumes from it.
No state is read across that boundary.
