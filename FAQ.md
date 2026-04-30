# FAQs

## Why write this?

I currently have a need for a simple command for quickly producing files using a template. I work within an engineering team
that is heavily investied in Terraform for their infrastructure-as-code tooling. This means that primarily the files I am
wanting to template are Terraform files, mostly [dynamic providers for modules](https://github.com/hashicorp/terraform/issues/24476).

Of course for the above problem I could just use [OpenTofu](https://opentofu.org/docs/language/providers/configuration/#for_each-multiple-instances-of-a-provider-configuration),
but this becomes a heated discussion due to the heavy investment into Terraform. I am also not interested in just
templating Terraform files, a single configuration source may be used to configure Helm values files, Ansible, etc.

Either way, I wanted a tool to use within a project to keep it DRY and dynamic.

## Why configure with HCL

As mentioned, I work within an engineering team heavily invested in Terraform. I wanted this tool to be configurable
in a way that is familiar and comfortable for the rest of the team.

## Why Jinja2 templates?

It is a text-based template language and thus can be used to generate any markup as well as source code.

## Why not use another tool?

I have seen [Terraplate](https://terraplate.verifa.io), but this is specifically targetting Terraform files and appears
to be a wrapper to the Terraform command.

I am also trying to avoid the use of [Terragrunt](https://terragrunt.com) as you become dependent on Terragrunt to run
Terraform, and the primary problem that the engineering team is trying to solve is dynamic providers. It is like
trying to crack a nut with a nuke in our context.

OpenTofu - yes, the problem is solved here, but we're not in a position to comfortably migrate.

## Why `jordplate`?

I liked the name `terraplate`, but this was already taken, and I wanted to distance the project from Terraform as the
tool will ultimately not be limited to templating Terraform.

I took the word `terra`, the Latin/Romance word for "Earth", added some distance by travelling from Southern Europe
by looking at the Northern European Germanic equivalent and picked the word `jord`.

It's still just "earth-plate" (think plate tectonics).

## How to pronounce `jordplate`?

For the Anglophones, it's pronounces a bit like "_your(d)-platter_", but "your plate" or "yord plate" will do.

## Was this written with AI?

Darn right it was. My weekend time is limited and I have a young family. I used Claude-code so please keep this in
mind when using. I have manually reviewed the code quickly, it's been built using the libraries I would normally pick
for a CLI utility (eg. "cobra"), but there's probably stuff I have glossed over.

## Can I contribute?

You sure can.
