package pike

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/rs/zerolog/log"
)

const (
	providerAWS    = "aws"
	providerAzure  = "azurerm"
	providerGoogle = "google"
	providerGCP    = "gcp"
)

type fileStringEmptyError struct{}

func (e *fileStringEmptyError) Error() string {
	return "no file provided"
}

// GetResources retrieves all the resources in a tf file.
func GetResources(file string, dirName string) ([]ResourceV2, error) {
	var Resources []ResourceV2

	if file == "" {
		return Resources, &fileStringEmptyError{}
	}

	temp, err := GetResourceBlocks(file)
	if err != nil {
		return Resources, err
	}

	moduleJson, err := GetModuleJson(filepath.Join(dirName, "/", ".terraform", "modules"))
	if err != nil {
		return Resources, err
	}

	for _, block := range temp.Blocks {
		var resource ResourceV2
		resource.TypeName = block.Type

		switch block.Type {
		case terraform:
			{
				Resources, _ = DetectBackend(resource, block, Resources)

				continue
			}
		case module:
			{
				LocalResources, err := GetLocalModules(block, dirName, moduleJson)
				if err == nil {
					Resources = append(LocalResources, Resources...)
				} else {
					log.Info().Msg(err.Error())
				}
			}
		case "output", "variable", "locals", "provider", "import":
			{
				continue
			}
		default:
			{
				// currently missed
			}
		}

		if block.Labels != nil {
			resource.Name = block.Labels[0]

			if len(block.Labels) > 1 {
				resource.ResourceName = block.Labels[1]
			}
		}

		var attributes []string

		resource.Attributes = GetBlockAttributes(attributes, block)
		if len(block.Labels) > 0 {
			resource.Provider = GetHCLType(block.Labels[0])
		} else {
			resource.Provider = "unknown"

			log.Info().Msgf("parsing error for %s", block.Type)
		}

		Resources = append(Resources, resource)
	}

	return Resources, nil
}

// DetectBackend handles permissions for backend blocks.
func DetectBackend(resource ResourceV2, block *hclsyntax.Block, resources []ResourceV2) ([]ResourceV2, error) {
	if resource.TypeName == terraform {
		if block.Body != nil && block.Body.Blocks != nil {
			for _, terraform := range block.Body.Blocks {
				if terraform.Type == "backend" {
					if terraform.Labels != nil && terraform.Labels[0] == "s3" {
						resource.Name = "backend"
						resource.Provider = providerAWS
						resource.Attributes = []string{"s3"}
						resources = append(resources, resource)

						return resources, nil
					}

					if terraform.Labels != nil && terraform.Labels[0] == "gcs" {
						resource.Name = "backend"
						resource.Provider = providerGCP
						resource.Attributes = []string{"gcs"}
						resources = append(resources, resource)

						return resources, nil
					}

				}
			}
		}
	}

	return nil, &backendExistsError{}
}

// GetResourceBlocks breaks down a file into resources.
func GetResourceBlocks(file string) (*hclsyntax.Body, error) {
	temp, err := filepath.Abs(file)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	src, err := os.ReadFile(temp)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	parser := hclparse.NewParser()
	parsedFile, fileDiags := parser.ParseHCL(src, file)

	if fileDiags != nil {
		return nil, fileDiags
	}

	return parsedFile.Body.(*hclsyntax.Body), err
}

// GetLocalModules return resource from a path.
func GetLocalModules(block *hclsyntax.Block, dirName string, listModulesJson ModuleJson) ([]ResourceV2, error) {
	var Resources []ResourceV2

	modulePath := GetModulePath(block)

	// not local
	if strings.Contains(modulePath, "git::") {
		return nil, &gitReferenceError{modulePath}
	}

	// Module is coming from HCP Terraform Cloud
	if strings.HasPrefix(modulePath, "app.terraform.io") {
		modulePath = ReturnLocalAddrFromSource(modulePath, listModulesJson)
	}
	// have the path to the module
	modulePath = filepath.Join(dirName, modulePath)
	// now process these extras
	ExtraFiles, err := GetTF(modulePath)
	if err != nil {
		log.Info().Msgf("local module scan getTF, %s", err)
	}

	for _, file := range ExtraFiles {
		resource, err := GetResources(file, dirName)
		if err == nil {
			Resources = append(Resources, resource...)
		}
	}

	return Resources, nil
}

// GetModulePath extracts the source location from a module.
func GetModulePath(block *hclsyntax.Block) string {
	var modulePath string

	attributes := block.Body.Attributes
	value := attributes["source"].Expr

	castValue, ok := value.(*hclsyntax.TemplateExpr)

	if !ok {
		return ""
	}

	parts := castValue.Parts

	for _, part := range parts {
		myPart, ok := part.(*hclsyntax.LiteralValueExpr)

		if !ok {
			return ""
		}

		modulePath = myPart.Val.AsString()
	}

	return modulePath
}

// GetBlockAttributes walks through a blocks getting all blocks and attributes.
func GetBlockAttributes(attributes []string, block *hclsyntax.Block) []string {
	for _, attribute := range block.Body.Attributes {
		attributes = append(attributes, attribute.Name)
	}

	for _, block := range block.Body.Blocks {
		// Also add in block names

		switch block.Type {
		case "dynamic":
			{
				attributes = append(attributes, block.Labels...)
			}
		case "resource":
			{
				// do nothing
			}
		default:
			{
				attributes = append(attributes, block.Type)
				attributes = GetBlockAttributes(attributes, block)
			}
		}
	}

	return attributes
}

// GetPermission determines the IAM permissions required and returns a list of permission.
func GetPermission(result ResourceV2) (Sorted, error) {
	var err error

	var myPermission Sorted

	switch result.Provider {
	case providerAWS:
		myPermission.AWS, err = GetAWSPermissions(result)
		if err != nil {
			log.Print(err)
		}
	case "oci", "digitalocean", "linode", "helm":
		log.Printf("Provider %s not yet implemented", result.Provider)

		return myPermission, nil
	case providerAzure, "azuread":
		myPermission.AZURE, err = GetAZUREPermissions(result)
		if err != nil {
			log.Print(err)
		}
	case providerGoogle, providerGCP:
		myPermission.GCP, err = getGCPPermissions(result)
		if err != nil {
			log.Print(err)
		}
	case "provider", "random", "main", "ip", "http", "test", "local",
		"archive", "tls", "template", "null", "time", "external", "kubernetes",
		"healthchecksio":
		return myPermission, nil
	default:
		if result.Provider != "" && !(result.TypeName == "module") {
			log.Info().Msgf("Provider %s was not found", result.Provider)
		} else {
			log.Info().Msgf("Provider %s Type %s not found", result.Provider, result.TypeName)
		}
	}

	return myPermission, err
}

func GetModuleJson(dir string) (ModuleJson, error) {
	return ReadModuleJsonForDir(dir)
}
