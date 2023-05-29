# Terrafile

Terrafile is a binary written in Go to retrieve modules from Azure DevOps for use in Terraform.

It is a non-backwards compatible fork of <https://github.com/coretech/terrafile>.

## How to install

### macOS

```sh
brew tap jwoffindin/terrafile && brew install terrafile
```

### Linux
Download your preferred flavor from the [releases](https://github.com/jwoffindin/terrafile/releases/latest) page and install manually.

For example:
```sh
curl -L https://github.com/jwoffindin/terrafile/releases/download/v{VERSION}/terrafile_{VERSION}_Linux_x86_64.tar.gz | tar xz -C /usr/local/bin
```

## How to use
Terrafile expects a file named `Terrafile` which will contain your terraform module dependencies in a yaml like format.

There are two approaches that can be used for managing your modules depending on the structure of your terraform code:
1. The default approach: `Terrafile` is located directly in the directory where terraform is run
2. Centrally managed: `Terrafile` is located in "root" directory of your terraform code, managing modules in all subfolders / stacks

### Default Approach
An example of default approach (#1) to `Terrafile`
```
tf-aws-vpc:
    source:  "git@github.com:terraform-aws-modules/terraform-aws-vpc"
    version: "v1.46.0"
tf-aws-vpc-experimental:
    source:  "git@github.com:terraform-aws-modules/terraform-aws-vpc"
    version: "master"
```

Terrafile config file in current directory and modules exported to ./vendor/modules
```sh
$ terrafile
INFO[0000] [*] Checking out v1.46.0 of git@github.com:terraform-aws-modules/terraform-aws-vpc
INFO[0000] [*] Checking out master of git@github.com:terraform-aws-modules/terraform-aws-vpc
```

Terrafile config file in custom directory
```sh
$ terrafile -f config/Terrafile
INFO[0000] [*] Checking out v1.46.0 of git@github.com:terraform-aws-modules/terraform-aws-vpc
INFO[0000] [*] Checking out master of git@github.com:terraform-aws-modules/terraform-aws-vpc
```

Terraform modules exported to custom directory
```sh
$ terrafile -p custom_directory
INFO[0000] [*] Checking out master of git@github.com:terraform-aws-modules/terraform-aws-vpc
INFO[0001] [*] Checking out v1.46.0 of git@github.com:terraform-aws-modules/terraform-aws-vpc
```

### Centrally Managed Approach
An example of using `Terrafile` in a root directory (#2):

Let's assume the following directory structure:

```
.
├── iam
│   ├── main.tf
│   └── .....tf
├── networking
│   ├── main.tf
│   └── .....tf
├── onboarding
.
.
.
├── some-other-stack
└── Terrafile
```

In the above scenario, Terrafile is not in every single folder but in the "root" of terraform code.

An example usage of centrally managed modules:

```
tf-aws-vpc:
    source:  "git@github.com:terraform-aws-modules/terraform-aws-vpc"
    version: "v1.46.0"
    destination:
        - networking
tf-aws-iam:
    source:  "git@github.com:terraform-aws-modules/terraform-aws-iam"
    version: "v5.11.1"
    destination:
        - iam
tf-aws-s3-bucket:
    source:  "git@github.com:terraform-aws-modules/terraform-aws-s3-bucket"
    version: "v3.6.1"
    destination:
        - networking
        - onboarding
        - some-other-stack
```

The `destination` of module is an array of directories (stacks) where the module should be used.
The module itself is fetched once and copied over to designated destinations.
Final destination of the module is handled in a similar way as in first approach: `$destination/$module_path/$module_key`.

The output of the run is exactly the same in both options.

## TODO
* Break out the main logic into separate commands (e.g. version, help, run)
* Update tests to include unit tests for broken out commands
* Add coverage tool and badge
* May be worth renaming Terrafile config file to something that won't be misinterpreted as the binary
