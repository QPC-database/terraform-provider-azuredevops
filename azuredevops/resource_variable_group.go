package azuredevops

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/microsoft/azure-devops-go-api/azuredevops/taskagent"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/utils/converter"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/utils/tfhelper"
	"github.com/microsoft/terraform-provider-azuredevops/azuredevops/utils/validate"
)

func resourceVariableGroup() *schema.Resource {
	return &schema.Resource{
		Create: resourceVariableGroupCreate,
		Read:   resourceVariableGroupRead,
		Update: resourceVariableGroupUpdate,
		Delete: resourceVariableGroupDelete,

		Schema: map[string]*schema.Schema{
			"project_id": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.UUID,
			},
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validate.NoEmptyStrings,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "",
			},
			// Not supported by API: https://github.com/microsoft/terraform-provider-azuredevops/issues/200
			// "allow_access": {
			// 	Type:     schema.TypeBool,
			// 	Required: true,
			// },
			"variable": {
				Type: schema.TypeSet,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Required: true,
						},
						"value": {
							Type:     schema.TypeString,
							Optional: true,
							Default:  "",
						},
						"is_secret": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
					},
				},
				Required: true,
				MinItems: 1,
				Set: func(i interface{}) int {
					item := i.(map[string]interface{})
					return schema.HashString(item["name"].(string))
				},
			},
		},
	}
}

func resourceVariableGroupCreate(d *schema.ResourceData, m interface{}) error {
	clients := m.(*aggregatedClient)
	variableGroupParameters, projectID := expandVariableGroupParameters(d)

	addedVariableGroup, err := createVariableGroup(clients, variableGroupParameters, projectID)
	if err != nil {
		return fmt.Errorf("Error creating variable group in Azure DevOps: %+v", err)
	}

	flattenVariableGroup(d, addedVariableGroup, projectID)
	return nil
}

func resourceVariableGroupRead(d *schema.ResourceData, m interface{}) error {
	clients := m.(*aggregatedClient)

	projectID, variableGroupID, err := tfhelper.ParseProjectIdAndResourceId(d)
	if err != nil {
		return fmt.Errorf("Error parsing the variable group ID from the Terraform resource data: %v", err)
	}

	variableGroup, err := clients.TaskAgentClient.GetVariableGroup(
		clients.ctx,
		taskagent.GetVariableGroupArgs{
			GroupId: &variableGroupID,
			Project: &projectID,
		},
	)
	if err != nil {
		return fmt.Errorf("Error looking up variable group given ID (%v) and project ID (%v): %v", variableGroupID, projectID, err)
	}

	flattenVariableGroup(d, variableGroup, &projectID)
	return nil
}

func resourceVariableGroupUpdate(d *schema.ResourceData, m interface{}) error {
	clients := m.(*aggregatedClient)
	variableGroupParams, projectID := expandVariableGroupParameters(d)

	_, variableGroupID, err := tfhelper.ParseProjectIdAndResourceId(d)
	if err != nil {
		return fmt.Errorf("Error parsing the variable group ID from the Terraform resource data: %v", err)
	}

	updatedVariableGroup, err := updateVariableGroup(clients, variableGroupParams, &variableGroupID, projectID)
	if err != nil {
		return fmt.Errorf("Error updating variable group in Azure DevOps: %+v", err)
	}

	flattenVariableGroup(d, updatedVariableGroup, projectID)
	return nil
}

func resourceVariableGroupDelete(d *schema.ResourceData, m interface{}) error {
	clients := m.(*aggregatedClient)
	projectID, variableGroupID, err := tfhelper.ParseProjectIdAndResourceId(d)
	if err != nil {
		return fmt.Errorf("Error parsing the variable group ID from the Terraform resource data: %v", err)
	}

	return deleteVariableGroup(clients, &projectID, &variableGroupID)
}

// Make the Azure DevOps API call to create the variable group
func createVariableGroup(clients *aggregatedClient, variableGroupParams *taskagent.VariableGroupParameters, project *string) (*taskagent.VariableGroup, error) {
	createdVariableGroup, err := clients.TaskAgentClient.AddVariableGroup(
		clients.ctx,
		taskagent.AddVariableGroupArgs{
			Group:   variableGroupParams,
			Project: project,
		})

	return createdVariableGroup, err
}

// Make the Azure DevOps API call to update the variable group
func updateVariableGroup(clients *aggregatedClient, parameters *taskagent.VariableGroupParameters, variableGroupID *int, project *string) (*taskagent.VariableGroup, error) {
	updatedVariableGroup, err := clients.TaskAgentClient.UpdateVariableGroup(
		clients.ctx,
		taskagent.UpdateVariableGroupArgs{
			Project: project,
			GroupId: variableGroupID,
			Group:   parameters,
		})

	return updatedVariableGroup, err
}

// Make the Azure DevOps API call to delete the variable group
func deleteVariableGroup(clients *aggregatedClient, project *string, variableGroupID *int) error {
	err := clients.TaskAgentClient.DeleteVariableGroup(
		clients.ctx,
		taskagent.DeleteVariableGroupArgs{
			Project: project,
			GroupId: variableGroupID,
		})

	return err
}

// Convert internal Terraform data structure to an AzDO data structure
func expandVariableGroupParameters(d *schema.ResourceData) (*taskagent.VariableGroupParameters, *string) {
	projectID := converter.String(d.Get("project_id").(string))
	variables := d.Get("variable").(*schema.Set).List()

	variableMap := make(map[string]taskagent.VariableValue)

	for _, variable := range variables {
		asMap := variable.(map[string]interface{})
		variableMap[asMap["name"].(string)] = taskagent.VariableValue{
			Value:    converter.String(asMap["value"].(string)),
			IsSecret: converter.Bool(asMap["is_secret"].(bool)),
		}
	}

	variableGroup := &taskagent.VariableGroupParameters{
		Name:        converter.String(d.Get("name").(string)),
		Description: converter.String(d.Get("description").(string)),
		Variables:   &variableMap,
	}

	return variableGroup, projectID
}

// Convert AzDO data structure to internal Terraform data structure
func flattenVariableGroup(d *schema.ResourceData, variableGroup *taskagent.VariableGroup, projectID *string) {
	d.SetId(fmt.Sprintf("%d", *variableGroup.Id))
	d.Set("name", *variableGroup.Name)
	d.Set("description", *variableGroup.Description)
	d.Set("variable", flattenVariables(variableGroup))
	d.Set("project_id", projectID)
}

// Convert AzDO Variables data structure to Terraform TypeSet
func flattenVariables(variableGroup *taskagent.VariableGroup) interface{} {
	// Preallocate list of variable prop maps
	variables := make([]map[string]interface{}, len(*variableGroup.Variables))

	index := 0
	for k, v := range *variableGroup.Variables {
		variables[index] = map[string]interface{}{
			"name":      k,
			"value":     converter.ToString(v.Value, ""),
			"is_secret": converter.ToBool(v.IsSecret, false),
		}
		index = index + 1
	}

	return variables
}