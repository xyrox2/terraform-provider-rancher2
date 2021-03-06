package rancher2

import (
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	clusterClient "github.com/rancher/types/client/cluster/v3"
)

func init() {
	descriptions = map[string]string{
		"name": "Name of the k8s namespace managed by rancher v2",

		"project_id": "Project ID where k8s namespace belongs",

		"project_name": "Project name where k8s namespace belongs",

		"description": "Description of the k8s namespace managed by rancher v2",

		"resource_quota_template_id": "Resource quota template id to apply on k8s namespace",

		"annotations": "Annotations of the k8s namespace managed by rancher v2",

		"labels": "Labels of the k8s namespace managed by rancher v2",
	}
}

//Schemas

func namespaceFields() map[string]*schema.Schema {
	s := map[string]*schema.Schema{
		"project_id": &schema.Schema{
			Type:        schema.TypeString,
			Required:    true,
			ForceNew:    true,
			Description: descriptions["project_id"],
		},
		"name": &schema.Schema{
			Type:        schema.TypeString,
			Required:    true,
			ForceNew:    true,
			Description: descriptions["name"],
		},
		"description": &schema.Schema{
			Type:        schema.TypeString,
			Optional:    true,
			Description: descriptions["description"],
		},
		"annotations": &schema.Schema{
			Type:        schema.TypeMap,
			Optional:    true,
			Computed:    true,
			Description: descriptions["annotations"],
		},
		"labels": &schema.Schema{
			Type:        schema.TypeMap,
			Optional:    true,
			Computed:    true,
			Description: descriptions["labels"],
		},
	}

	return s
}

// Flatteners

func flattenNamespace(d *schema.ResourceData, in *clusterClient.Namespace) error {
	if in == nil {
		return nil
	}

	d.SetId(in.ID)

	err := d.Set("project_id", in.ProjectID)
	if err != nil {
		return err
	}

	err = d.Set("name", in.Name)
	if err != nil {
		return err
	}

	err = d.Set("description", in.Description)
	if err != nil {
		return err
	}

	err = d.Set("annotations", toMapInterface(in.Annotations))
	if err != nil {
		return err
	}

	err = d.Set("labels", toMapInterface(in.Labels))
	if err != nil {
		return err
	}

	return nil

}

// Expanders

func expandNamespace(in *schema.ResourceData) *clusterClient.Namespace {
	obj := &clusterClient.Namespace{}
	if in == nil {
		return nil
	}

	if v := in.Id(); len(v) > 0 {
		obj.ID = v
	}

	obj.ProjectID = in.Get("project_id").(string)
	obj.Name = in.Get("name").(string)
	obj.Description = in.Get("description").(string)

	if v, ok := in.Get("annotations").(map[string]interface{}); ok && len(v) > 0 {
		obj.Annotations = toMapString(v)
	}

	if v, ok := in.Get("labels").(map[string]interface{}); ok && len(v) > 0 {
		obj.Labels = toMapString(v)
	}

	return obj
}

func resourceRancher2Namespace() *schema.Resource {
	return &schema.Resource{
		Create: resourceRancher2NamespaceCreate,
		Read:   resourceRancher2NamespaceRead,
		Update: resourceRancher2NamespaceUpdate,
		Delete: resourceRancher2NamespaceDelete,
		Importer: &schema.ResourceImporter{
			State: resourceRancher2NamespaceImport,
		},

		Schema: namespaceFields(),
	}
}

func resourceRancher2NamespaceCreate(d *schema.ResourceData, meta interface{}) error {
	clusterID, err := clusterIDFromProjectID(d.Get("project_id").(string))
	if err != nil {
		return err
	}

	active, err := meta.(*Config).isClusterActive(clusterID)
	if err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("[ERROR] Creating namespace: Cluster ID %s is not active", clusterID)
	}

	client, err := meta.(*Config).ClusterClient(clusterID)
	if err != nil {
		return err
	}

	ns := expandNamespace(d)

	log.Printf("[INFO] Creating Namespace %s on Cluster ID %s", ns.Name, clusterID)

	newNs, err := client.Namespace.Create(ns)
	if err != nil {
		return err
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"activating"},
		Target:     []string{"active"},
		Refresh:    namespaceStateRefreshFunc(client, newNs.ID),
		Timeout:    10 * time.Minute,
		Delay:      1 * time.Second,
		MinTimeout: 3 * time.Second,
	}
	_, waitErr := stateConf.WaitForState()
	if waitErr != nil {
		return fmt.Errorf(
			"[ERROR] waiting for namespace (%s) to be created: %s", newNs.ID, waitErr)
	}

	err = flattenNamespace(d, newNs)
	if err != nil {
		return err
	}

	return resourceRancher2NamespaceRead(d, meta)
}

func resourceRancher2NamespaceRead(d *schema.ResourceData, meta interface{}) error {
	clusterID, err := clusterIDFromProjectID(d.Get("project_id").(string))
	if err != nil {
		return err
	}

	log.Printf("[INFO] Refreshing Namespace ID %s", d.Id())

	client, err := meta.(*Config).ClusterClient(clusterID)
	if err != nil {
		return err
	}

	ns, err := client.Namespace.ByID(d.Id())
	if err != nil {
		if IsNotFound(err) {
			log.Printf("[INFO] Namespace ID %s not found.", d.Id())
			d.SetId("")
			return nil
		}
		return err
	}

	err = flattenNamespace(d, ns)
	if err != nil {
		return err
	}

	return nil
}

func resourceRancher2NamespaceUpdate(d *schema.ResourceData, meta interface{}) error {
	clusterID, err := clusterIDFromProjectID(d.Get("project_id").(string))
	if err != nil {
		return err
	}

	log.Printf("[INFO] Updating Namespace ID %s", d.Id())

	client, err := meta.(*Config).ClusterClient(clusterID)
	if err != nil {
		return err
	}

	ns, err := client.Namespace.ByID(d.Id())
	if err != nil {
		return err
	}

	update := map[string]interface{}{
		"projectId":   d.Get("project_id").(string),
		"description": d.Get("description").(string),
		"annotations": toMapString(d.Get("annotations").(map[string]interface{})),
		"labels":      toMapString(d.Get("labels").(map[string]interface{})),
	}

	newNs, err := client.Namespace.Update(ns, update)
	if err != nil {
		return err
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"active"},
		Target:     []string{"active"},
		Refresh:    namespaceStateRefreshFunc(client, newNs.ID),
		Timeout:    10 * time.Minute,
		Delay:      1 * time.Second,
		MinTimeout: 3 * time.Second,
	}
	_, waitErr := stateConf.WaitForState()
	if waitErr != nil {
		return fmt.Errorf(
			"[ERROR] waiting for namespace (%s) to be updated: %s", newNs.ID, waitErr)
	}

	err = flattenNamespace(d, newNs)
	if err != nil {
		return err
	}

	return resourceRancher2NamespaceRead(d, meta)
}

func resourceRancher2NamespaceDelete(d *schema.ResourceData, meta interface{}) error {
	clusterID, err := clusterIDFromProjectID(d.Get("project_id").(string))
	if err != nil {
		return err
	}

	log.Printf("[INFO] Deleting Namespace ID %s", d.Id())
	id := d.Id()
	client, err := meta.(*Config).ClusterClient(clusterID)
	if err != nil {
		return err
	}

	ns, err := client.Namespace.ByID(id)
	if err != nil {
		if IsNotFound(err) {
			log.Printf("[INFO] Namespace ID %s not found.", d.Id())
			d.SetId("")
			return nil
		}
		return err
	}

	err = client.Namespace.Delete(ns)
	if err != nil {
		return fmt.Errorf("Error removing Namespace: %s", err)
	}

	log.Printf("[DEBUG] Waiting for namespace (%s) to be removed", id)

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"removing"},
		Target:     []string{"removed"},
		Refresh:    namespaceStateRefreshFunc(client, id),
		Timeout:    10 * time.Minute,
		Delay:      1 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, waitErr := stateConf.WaitForState()
	if waitErr != nil {
		return fmt.Errorf(
			"[ERROR] waiting for namespace (%s) to be removed: %s", id, waitErr)
	}

	d.SetId("")
	return nil
}

func resourceRancher2NamespaceImport(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	clusterID, resourceID := splitID(d.Id())

	client, err := meta.(*Config).ClusterClient(clusterID)
	if err != nil {
		return []*schema.ResourceData{}, err
	}
	ns, err := client.Namespace.ByID(resourceID)
	if err != nil {
		return []*schema.ResourceData{}, err
	}

	err = flattenNamespace(d, ns)
	if err != nil {
		return []*schema.ResourceData{}, err
	}

	return []*schema.ResourceData{d}, nil
}

// namespaceStateRefreshFunc returns a resource.StateRefreshFunc, used to watch a Rancher Namespace.
func namespaceStateRefreshFunc(client *clusterClient.Client, nsID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		obj, err := client.Namespace.ByID(nsID)
		if err != nil {
			if IsNotFound(err) {
				return obj, "removed", nil
			}
			return nil, "", err
		}

		if obj.Removed != "" {
			return obj, "removed", nil
		}

		return obj, obj.State, nil
	}
}
