// For more info about writing custom provider: https://www.terraform.io/docs/extend/writing-custom-providers.html

package datadog

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	_nethttp "net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/terraform-providers/terraform-provider-datadog/datadog/internal/utils"
	"github.com/terraform-providers/terraform-provider-datadog/datadog/internal/validators"

	datadogV1 "github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceDatadogSyntheticsTest() *schema.Resource {
	return &schema.Resource{
		Description:   "Provides a Datadog synthetics test resource. This can be used to create and manage Datadog synthetics test.",
		CreateContext: resourceDatadogSyntheticsTestCreate,
		ReadContext:   resourceDatadogSyntheticsTestRead,
		UpdateContext: resourceDatadogSyntheticsTestUpdate,
		DeleteContext: resourceDatadogSyntheticsTestDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"type": {
				Description:      "Synthetics test type.",
				Type:             schema.TypeString,
				Required:         true,
				ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsTestDetailsTypeFromValue),
			},
			"subtype": {
				Description: "The subtype of the Synthetic API test. Defaults to `http`.",
				Type:        schema.TypeString,
				Optional:    true,
				DiffSuppressFunc: func(key, old, new string, d *schema.ResourceData) bool {
					if d.Get("type") == "api" && old == "http" && new == "" {
						// defaults to http if type is api for retro-compatibility
						return true
					}
					return old == new
				},
				ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsTestDetailsSubTypeFromValue),
			},
			"request_definition": {
				Description: "Required if `type = \"api\"`. The synthetics test request.",
				Type:        schema.TypeList,
				MaxItems:    1,
				Optional:    true,
				Elem:        syntheticsTestRequest(),
			},
			"request_headers":            syntheticsTestRequestHeaders(),
			"request_query":              syntheticsTestRequestQuery(),
			"request_basicauth":          syntheticsTestRequestBasicAuth(),
			"request_proxy":              syntheticsTestRequestProxy(),
			"request_client_certificate": syntheticsTestRequestClientCertificate(),
			"assertion":                  syntheticsAPIAssertion(),
			"browser_variable":           syntheticsBrowserVariable(),
			"config_variable":            syntheticsConfigVariable(),
			"device_ids": {
				Description: "Required if `type = \"browser\"`. Array with the different device IDs used to run the test.",
				Type:        schema.TypeList,
				Optional:    true,
				Elem: &schema.Schema{
					Type:             schema.TypeString,
					ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsDeviceIDFromValue),
				},
			},
			"locations": {
				Description: "Array of locations used to run the test. Refer to [the Datadog Synthetics location data source](https://registry.terraform.io/providers/DataDog/datadog/latest/docs/data-sources/synthetics_locations) to retrieve the list of locations.",
				Type:        schema.TypeSet,
				Required:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"options_list": syntheticsTestOptionsList(),
			"name": {
				Description: "Name of Datadog synthetics test.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"message": {
				Description: "A message to include with notifications for this synthetics test. Email notifications can be sent to specific users by using the same `@username` notation as events.",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
			},
			"tags": {
				Description: "A list of tags to associate with your synthetics test. This can help you categorize and filter tests in the manage synthetics page of the UI. Default is an empty list (`[]`).",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"status": {
				Description:      "Define whether you want to start (`live`) or pause (`paused`) a Synthetic test.",
				Type:             schema.TypeString,
				Required:         true,
				ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsTestPauseStatusFromValue),
			},
			"monitor_id": {
				Description: "ID of the monitor associated with the Datadog synthetics test.",
				Type:        schema.TypeInt,
				Computed:    true,
			},
			"browser_step": syntheticsTestBrowserStep(),
			"api_step":     syntheticsTestAPIStep(),
			"set_cookie": {
				Description: "Cookies to be used for a browser test request, using the [Set-Cookie](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Set-Cookie) syntax.",
				Type:        schema.TypeString,
				Optional:    true,
			},
		},
	}
}

func syntheticsTestRequest() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"method": {
				Description:      "The HTTP method.",
				Type:             schema.TypeString,
				Optional:         true,
				ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewHTTPMethodFromValue),
			},
			"url": {
				Description: "The URL to send the request to.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"body": {
				Description: "The request body.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"timeout": {
				Description: "Timeout in seconds for the test. Defaults to `60`.",
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     60,
			},
			"host": {
				Description: "Host name to perform the test with.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"port": {
				Description: "Port to use when performing the test.",
				Type:        schema.TypeInt,
				Optional:    true,
			},
			"dns_server": {
				Description: "DNS server to use for DNS tests (`subtype = \"dns\"`).",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"dns_server_port": {
				Description:  "DNS server port to use for DNS tests.",
				Type:         schema.TypeInt,
				Optional:     true,
				ValidateFunc: validation.IntAtMost(65535),
			},
			"no_saving_response_body": {
				Description: "Determines whether or not to save the response body.",
				Type:        schema.TypeBool,
				Optional:    true,
			},
			"number_of_packets": {
				Description:  "Number of pings to use per test for ICMP tests (`subtype = \"icmp\"`) between 0 and 10.",
				Type:         schema.TypeInt,
				Optional:     true,
				ValidateFunc: validation.IntBetween(0, 10),
			},
			"should_track_hops": {
				Description: "This will turn on a traceroute probe to discover all gateways along the path to the host destination. For ICMP tests (`subtype = \"icmp\"`).",
				Type:        schema.TypeBool,
				Optional:    true,
			},
			"servername": {
				Description: "For SSL tests, it specifies on which server you want to initiate the TLS handshake, allowing the server to present one of multiple possible certificates on the same IP address and TCP port number.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"message": {
				Description: "For UDP and websocket tests, message to send with the request.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"service": {
				Description: "For gRPC tests, service to target for healthcheck.",
				Type:        schema.TypeString,
				Optional:    true,
			},
		},
	}
}

func syntheticsTestRequestHeaders() *schema.Schema {
	return &schema.Schema{
		Description: "Header name and value map.",
		Type:        schema.TypeMap,
		Optional:    true,
	}
}

func syntheticsTestRequestQuery() *schema.Schema {
	return &schema.Schema{
		Description: "Query arguments name and value map.",
		Type:        schema.TypeMap,
		Optional:    true,
	}
}

func syntheticsTestRequestBasicAuth() *schema.Schema {
	return &schema.Schema{
		Description: "The HTTP basic authentication credentials. Exactly one nested block is allowed with the structure below.",
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"type": {
					Description:  "Type of basic authentication to use when performing the test.",
					Type:         schema.TypeString,
					Optional:     true,
					Default:      "web",
					ValidateFunc: validation.StringInSlice([]string{"web", "sigv4", "ntlm"}, false),
				},
				"username": {
					Description: "Username for authentication.",
					Type:        schema.TypeString,
					Optional:    true,
				},
				"password": {
					Description: "Password for authentication.",
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
				},
				"access_key": {
					Type:        schema.TypeString,
					Description: "Access key for `SIGV4` authentication.",
					Optional:    true,
					Sensitive:   true,
				},
				"secret_key": {
					Type:        schema.TypeString,
					Description: "Secret key for `SIGV4` authentication.",
					Optional:    true,
					Sensitive:   true,
				},
				"region": {
					Type:        schema.TypeString,
					Description: "Region for `SIGV4` authentication.",
					Optional:    true,
				},
				"service_name": {
					Type:        schema.TypeString,
					Description: "Service name for `SIGV4` authentication.",
					Optional:    true,
				},
				"session_token": {
					Type:        schema.TypeString,
					Description: "Session token for `SIGV4` authentication.",
					Optional:    true,
				},
				"domain": {
					Type:        schema.TypeString,
					Description: "Domain for `ntlm` authentication.",
					Optional:    true,
				},
				"workstation": {
					Type:        schema.TypeString,
					Description: "Workstation for `ntlm` authentication.",
					Optional:    true,
				},
			},
		},
	}
}

func syntheticsTestRequestProxy() *schema.Schema {
	return &schema.Schema{
		Description: "The proxy to perform the test.",
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"url": {
					Type:        schema.TypeString,
					Description: "URL of the proxy to perform the test.",
					Required:    true,
				},
				"headers": syntheticsTestRequestHeaders(),
			},
		},
	}
}

func syntheticsTestRequestClientCertificate() *schema.Schema {
	return &schema.Schema{
		Description: "Client certificate to use when performing the test request. Exactly one nested block is allowed with the structure below.",
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"cert": syntheticsTestRequestClientCertificateItem(),
				"key":  syntheticsTestRequestClientCertificateItem(),
			},
		},
	}
}

func syntheticsTestRequestClientCertificateItem() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		MaxItems: 1,
		Required: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"content": {
					Description: "Content of the certificate.",
					Type:        schema.TypeString,
					Required:    true,
					Sensitive:   true,
					StateFunc: func(val interface{}) string {
						return utils.ConvertToSha256(val.(string))
					},
				},
				"filename": {
					Description: "File name for the certificate.",
					Type:        schema.TypeString,
					Optional:    true,
					Default:     "Provided in Terraform config",
				},
			},
		},
	}
}

func syntheticsAPIAssertion() *schema.Schema {
	return &schema.Schema{
		Description: "Assertions used for the test. Multiple `assertion` blocks are allowed with the structure below.",
		Type:        schema.TypeList,
		Optional:    true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"type": {
					Description:      "Type of assertion. **Note** Only some combinations of `type` and `operator` are valid (please refer to [Datadog documentation](https://docs.datadoghq.com/api/latest/synthetics/#create-a-test)).",
					Type:             schema.TypeString,
					ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsAssertionTypeFromValue),
					Required:         true,
				},
				"operator": {
					Description:  "Assertion operator. **Note** Only some combinations of `type` and `operator` are valid (please refer to [Datadog documentation](https://docs.datadoghq.com/api/latest/synthetics/#create-a-test)).",
					Type:         schema.TypeString,
					Required:     true,
					ValidateFunc: validateSyntheticsAssertionOperator,
				},
				"property": {
					Description: "If assertion type is `header`, this is the header name.",
					Type:        schema.TypeString,
					Optional:    true,
				},
				"target": {
					Description: "Expected value. Depends on the assertion type, refer to [Datadog documentation](https://docs.datadoghq.com/api/latest/synthetics/#create-a-test) for details.",
					Type:        schema.TypeString,
					Optional:    true,
				},
				"targetjsonpath": {
					Description: "Expected structure if `operator` is `validatesJSONPath`. Exactly one nested block is allowed with the structure below.",
					Type:        schema.TypeList,
					Optional:    true,
					MaxItems:    1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"operator": {
								Description: "The specific operator to use on the path.",
								Type:        schema.TypeString,
								Required:    true,
							},
							"jsonpath": {
								Description: "The JSON path to assert.",
								Type:        schema.TypeString,
								Required:    true,
							},
							"targetvalue": {
								Description: "Expected matching value.",
								Type:        schema.TypeString,
								Required:    true,
							},
						},
					},
				},
			},
		},
	}
}

func syntheticsTestOptionsRetry() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		MaxItems: 1,
		Optional: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"count": {
					Description: "Number of retries needed to consider a location as failed before sending a notification alert.",
					Type:        schema.TypeInt,
					Default:     0,
					Optional:    true,
				},
				"interval": {
					Description: "Interval between a failed test and the next retry in milliseconds.",
					Type:        schema.TypeInt,
					Default:     300,
					Optional:    true,
				},
			},
		},
	}
}

func syntheticsTestOptionsList() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"allow_insecure":   syntheticsAllowInsecureOption(),
				"follow_redirects": syntheticsFollowRedirectsOption(),
				"tick_every": {
					Description:  "How often the test should run (in seconds).",
					Type:         schema.TypeInt,
					Required:     true,
					ValidateFunc: validation.IntBetween(30, 604800),
				},
				"accept_self_signed": {
					Description: "For SSL test, whether or not the test should allow self signed certificates.",
					Type:        schema.TypeBool,
					Optional:    true,
				},
				"min_location_failed": {
					Description: "Minimum number of locations in failure required to trigger an alert. Default is `1`.",
					Type:        schema.TypeInt,
					Default:     1,
					Optional:    true,
				},
				"min_failure_duration": {
					Description: "Minimum amount of time in failure required to trigger an alert. Default is `0`.",
					Type:        schema.TypeInt,
					Optional:    true,
				},
				"monitor_name": {
					Description: "The monitor name is used for the alert title as well as for all monitor dashboard widgets and SLOs.",
					Type:        schema.TypeString,
					Optional:    true,
				},
				"monitor_options": {
					Type:     schema.TypeList,
					MaxItems: 1,
					Optional: true,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"renotify_interval": {
								Description: "Specify a renotification frequency.",
								Type:        schema.TypeInt,
								Default:     0,
								Optional:    true,
							},
						},
					},
				},
				"monitor_priority": {
					Type:         schema.TypeInt,
					Optional:     true,
					ValidateFunc: validation.IntBetween(1, 5),
				},
				"restricted_roles": {
					Description: "A list of role identifiers pulled from the Roles API to restrict read and write access.",
					Type:        schema.TypeSet,
					Optional:    true,
					Elem:        &schema.Schema{Type: schema.TypeString},
				},
				"retry": syntheticsTestOptionsRetry(),
				"no_screenshot": {
					Description: "Prevents saving screenshots of the steps.",
					Type:        schema.TypeBool,
					Optional:    true,
				},
				"check_certificate_revocation": {
					Description: "For SSL test, whether or not the test should fail on revoked certificate in stapled OCSP.",
					Type:        schema.TypeBool,
					Optional:    true,
				},
				"ci": {
					Description: "CI/CD options for a Synthetic test.",
					Type:        schema.TypeList,
					MaxItems:    1,
					Optional:    true,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"execution_rule": {
								Type:             schema.TypeString,
								Description:      "Execution rule for a Synthetics test.",
								ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsTestExecutionRuleFromValue),
								Optional:         true,
							},
						},
					},
				},
				"rum_settings": {
					Description: "The RUM data collection settings for the Synthetic browser test.",
					Type:        schema.TypeList,
					MaxItems:    1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"is_enabled": {
								Type:        schema.TypeBool,
								Description: "Determines whether RUM data is collected during test runs.",
								Required:    true,
							},
							"application_id": {
								Type:        schema.TypeString,
								Description: "RUM application ID used to collect RUM data for the browser test.",
								Optional:    true,
							},
							"client_token_id": {
								Type:        schema.TypeInt,
								Description: "RUM application API key ID used to collect RUM data for the browser test.",
								Sensitive:   true,
								Optional:    true,
							},
						},
					},
					Optional: true,
				},
			},
		},
	}
}

func syntheticsTestAPIStep() *schema.Schema {
	requestElemSchema := syntheticsTestRequest()
	requestElemSchema.Schema["allow_insecure"] = syntheticsAllowInsecureOption()
	requestElemSchema.Schema["follow_redirects"] = syntheticsFollowRedirectsOption()

	return &schema.Schema{
		Description: "Steps for multistep api tests",
		Type:        schema.TypeList,
		Optional:    true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"name": {
					Description: "The name of the step.",
					Type:        schema.TypeString,
					Required:    true,
				},
				"subtype": {
					Description:      "The subtype of the Synthetic multistep API test step.",
					Type:             schema.TypeString,
					Optional:         true,
					Default:          "http",
					ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsAPIStepSubtypeFromValue),
				},
				"extracted_value": {
					Description: "Values to parse and save as variables from the response.",
					Type:        schema.TypeList,
					Optional:    true,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"name": {
								Type:     schema.TypeString,
								Required: true,
							},
							"type": {
								Description:      "Property of the Synthetics Test Response to use for the variable.",
								Type:             schema.TypeString,
								Required:         true,
								ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsGlobalVariableParseTestOptionsTypeFromValue),
							},
							"field": {
								Description: "When type is `http_header`, name of the header to use to extract the value.",
								Type:        schema.TypeString,
								Optional:    true,
							},
							"parser": {
								Type:     schema.TypeList,
								MaxItems: 1,
								Required: true,
								Elem: &schema.Resource{
									Schema: map[string]*schema.Schema{
										"type": {
											Description:      "Type of parser for a Synthetics global variable from a synthetics test.",
											Type:             schema.TypeString,
											Required:         true,
											ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsGlobalVariableParserTypeFromValue),
										},
										"value": {
											Type:        schema.TypeString,
											Description: "Regex or JSON path used for the parser. Not used with type `raw`.",
											Optional:    true,
										},
									},
								},
							},
						},
					},
				},
				"request_definition": {
					Description: "The request for the api step.",
					Type:        schema.TypeList,
					MaxItems:    1,
					Optional:    true,
					Elem:        requestElemSchema,
				},
				"request_headers":            syntheticsTestRequestHeaders(),
				"request_query":              syntheticsTestRequestQuery(),
				"request_basicauth":          syntheticsTestRequestBasicAuth(),
				"request_proxy":              syntheticsTestRequestProxy(),
				"request_client_certificate": syntheticsTestRequestClientCertificate(),
				"assertion":                  syntheticsAPIAssertion(),
				"allow_failure": {
					Description: "Determines whether or not to continue with test if this step fails.",
					Type:        schema.TypeBool,
					Optional:    true,
				},
				"is_critical": {
					Description: "Determines whether or not to consider the entire test as failed if this step fails. Can be used only if `allow_failure` is `true`.",
					Type:        schema.TypeBool,
					Optional:    true,
				},
				"retry": syntheticsTestOptionsRetry(),
			},
		},
	}
}

func syntheticsTestBrowserStep() *schema.Schema {
	paramsSchema := syntheticsBrowserStepParams()
	browserStepSchema := schema.Schema{
		Description: "Steps for browser tests.",
		Type:        schema.TypeList,
		Optional:    true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"name": {
					Description: "Name of the step.",
					Type:        schema.TypeString,
					Required:    true,
				},
				"type": {
					Description:      "Type of the step.",
					Type:             schema.TypeString,
					Required:         true,
					ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsStepTypeFromValue),
				},
				"allow_failure": {
					Description: "Determines if the step should be allowed to fail.",
					Type:        schema.TypeBool,
					Optional:    true,
				},
				"is_critical": {
					Description: "Determines whether or not to consider the entire test as failed if this step fails. Can be used only if `allow_failure` is `true`.",
					Type:        schema.TypeBool,
					Optional:    true,
				},
				"timeout": {
					Description: "Used to override the default timeout of a step.",
					Type:        schema.TypeInt,
					Optional:    true,
				},
				"params": &paramsSchema,
				"force_element_update": {
					Description: `Force update of the "element" parameter for the step`,
					Type:        schema.TypeBool,
					Optional:    true,
				},
			},
		},
	}

	return &browserStepSchema
}

func syntheticsBrowserStepParams() schema.Schema {
	return schema.Schema{
		Description: "Parameters for the step.",
		Type:        schema.TypeList,
		MaxItems:    1,
		Required:    true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"attribute": {
					Description: `Name of the attribute to use for an "assert attribute" step.`,
					Type:        schema.TypeString,
					Optional:    true,
				},
				"check": {
					Description:      "Check type to use for an assertion step.",
					Type:             schema.TypeString,
					Optional:         true,
					ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsCheckTypeFromValue),
				},
				"click_type": {
					Description:  `Type of click to use for a "click" step.`,
					Type:         schema.TypeString,
					Optional:     true,
					ValidateFunc: validation.StringInSlice([]string{"contextual", "double", "primary"}, false),
				},
				"code": {
					Description: "Javascript code to use for the step.",
					Type:        schema.TypeString,
					Optional:    true,
				},
				"delay": {
					Description: `Delay between each key stroke for a "type test" step.`,
					Type:        schema.TypeInt,
					Optional:    true,
				},
				"element": {
					Description: "Element to use for the step, json encoded string.",
					Type:        schema.TypeString,
					Optional:    true,
					DiffSuppressFunc: func(key, old, new string, d *schema.ResourceData) bool {
						// if there is no old value we let TF handle this
						if old == "" {
							return old == new
						}

						forceElementUpdateKey := strings.Replace(key, "params.0.element", "force_element_update", 1)

						// if the field force_element_update is present we force the update
						// of the step params
						if attr, ok := d.GetOk(forceElementUpdateKey); ok && attr.(bool) {
							return false
						}

						// by default we ignore the diff for step parameters because some of them
						// are updated by the backend.
						return true
					},
				},
				"element_user_locator": {
					Description: "Custom user selector to use for the step.",
					Type:        schema.TypeList,
					MaxItems:    1,
					Optional:    true,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"fail_test_on_cannot_locate": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  false,
							},
							"value": {
								Type:     schema.TypeList,
								MaxItems: 1,
								Required: true,
								Elem: &schema.Resource{
									Schema: map[string]*schema.Schema{
										"type": {
											Type:         schema.TypeString,
											Optional:     true,
											Default:      "css",
											ValidateFunc: validation.StringInSlice([]string{"css", "xpath"}, false),
										},
										"value": {
											Type:     schema.TypeString,
											Required: true,
										},
									},
								},
							},
						},
					},
				},
				"email": {
					Description: `Details of the email for an "assert email" step.`,
					Type:        schema.TypeString,
					Optional:    true,
				},
				"file": {
					Description: `For an "assert download" step.`,
					Type:        schema.TypeString,
					Optional:    true,
				},
				"files": {
					Description: `Details of the files for an "upload files" step, json encoded string.`,
					Type:        schema.TypeString,
					Optional:    true,
				},
				"modifiers": {
					Description: `Modifier to use for a "press key" step.`,
					Type:        schema.TypeList,
					Optional:    true,
					Elem: &schema.Schema{
						Type:         schema.TypeString,
						ValidateFunc: validation.StringInSlice([]string{"Alt", "Control", "meta", "Shift"}, false),
					},
				},
				"playing_tab_id": {
					Description: "ID of the tab to play the subtest.",
					Type:        schema.TypeString,
					Optional:    true,
				},
				"request": {
					Description: "Request for an API step.",
					Type:        schema.TypeString,
					Optional:    true,
				},
				"subtest_public_id": {
					Description: "ID of the Synthetics test to use as subtest.",
					Type:        schema.TypeString,
					Optional:    true,
				},
				"value": {
					Description: "Value of the step.",
					Type:        schema.TypeString,
					Optional:    true,
				},
				"variable": {
					Description: "Details of the variable to extract.",
					Type:        schema.TypeList,
					MaxItems:    1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"name": {
								Description: "Name of the extracted variable.",
								Type:        schema.TypeString,
								Optional:    true,
							},
							"example": {
								Description: "Example of the extracted variable.",
								Default:     "",
								Type:        schema.TypeString,
								Optional:    true,
							},
						},
					},
					Optional: true,
				},
				"with_click": {
					Description: `For "file upload" steps.`,
					Type:        schema.TypeBool,
					Optional:    true,
				},
				"x": {
					Description: `X coordinates for a "scroll step".`,
					Type:        schema.TypeInt,
					Optional:    true,
				},
				"y": {
					Description: `Y coordinates for a "scroll step".`,
					Type:        schema.TypeInt,
					Optional:    true,
				},
			},
		},
	}
}

func syntheticsBrowserVariable() *schema.Schema {
	return &schema.Schema{
		Description: "Variables used for a browser test steps. Multiple `variable` blocks are allowed with the structure below.",
		Type:        schema.TypeList,
		Optional:    true,
		Elem:        syntheticsBrowserVariableElem(),
	}
}

func syntheticsBrowserVariableElem() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"example": {
				Description: "Example for the variable.",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
			},
			"id": {
				Description: "ID of the global variable to use. This is actually only used (and required) in the case of using a variable of type `global`.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"name": {
				Description:  "Name of the variable.",
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringMatch(regexp.MustCompile(`^[A-Z][A-Z0-9_]+[A-Z0-9]$`), "must be all uppercase with underscores"),
			},
			"pattern": {
				Description: "Pattern of the variable.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"type": {
				Description:      "Type of browser test variable.",
				Type:             schema.TypeString,
				Required:         true,
				ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsBrowserVariableTypeFromValue),
			},
		},
	}
}

func syntheticsConfigVariable() *schema.Schema {
	return &schema.Schema{
		Description: "Variables used for the test configuration. Multiple `config_variable` blocks are allowed with the structure below.",
		Type:        schema.TypeList,
		Optional:    true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"example": {
					Description: "Example for the variable.",
					Type:        schema.TypeString,
					Optional:    true,
				},
				"name": {
					Description:  "Name of the variable.",
					Type:         schema.TypeString,
					Required:     true,
					ValidateFunc: validation.StringMatch(regexp.MustCompile(`^[A-Z][A-Z0-9_]+[A-Z0-9]$`), "must be all uppercase with underscores"),
				},
				"pattern": {
					Description: "Pattern of the variable.",
					Type:        schema.TypeString,
					Optional:    true,
				},
				"type": {
					Description:      "Type of test configuration variable.",
					Type:             schema.TypeString,
					Required:         true,
					ValidateDiagFunc: validators.ValidateEnumValue(datadogV1.NewSyntheticsConfigVariableTypeFromValue),
				},
				"id": {
					Description: "When type = `global`, ID of the global variable to use.",
					Type:        schema.TypeString,
					Optional:    true,
				},
			},
		},
	}
}

func syntheticsAllowInsecureOption() *schema.Schema {
	return &schema.Schema{
		Description: "Allows loading insecure content for an HTTP test.",
		Type:        schema.TypeBool,
		Optional:    true,
	}
}

func syntheticsFollowRedirectsOption() *schema.Schema {
	return &schema.Schema{
		Description: "Determines whether or not the API HTTP test should follow redirects.",
		Type:        schema.TypeBool,
		Optional:    true,
	}
}

func resourceDatadogSyntheticsTestCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	providerConf := meta.(*ProviderConfiguration)
	datadogClientV1 := providerConf.DatadogClientV1
	authV1 := providerConf.AuthV1

	testType := getSyntheticsTestType(d)

	if testType == datadogV1.SYNTHETICSTESTDETAILSTYPE_API {
		syntheticsTest := buildSyntheticsAPITestStruct(d)
		createdSyntheticsTest, httpResponseCreate, err := datadogClientV1.SyntheticsApi.CreateSyntheticsAPITest(authV1, *syntheticsTest)
		if err != nil {
			// Note that Id won't be set, so no state will be saved.
			return utils.TranslateClientErrorDiag(err, httpResponseCreate, "error creating synthetics API test")
		}
		if err := utils.CheckForUnparsed(createdSyntheticsTest); err != nil {
			return diag.FromErr(err)
		}

		var getSyntheticsApiTestResponse datadogV1.SyntheticsAPITest
		var httpResponseGet *_nethttp.Response
		err = resource.RetryContext(ctx, d.Timeout(schema.TimeoutCreate), func() *resource.RetryError {
			getSyntheticsApiTestResponse, httpResponseGet, err = datadogClientV1.SyntheticsApi.GetAPITest(authV1, createdSyntheticsTest.GetPublicId())
			if err != nil {
				if httpResponseGet != nil && httpResponseGet.StatusCode == 404 {
					return resource.RetryableError(fmt.Errorf("synthetics api test not created yet"))
				}

				return resource.NonRetryableError(err)
			}
			if err := utils.CheckForUnparsed(getSyntheticsApiTestResponse); err != nil {
				return resource.NonRetryableError(err)
			}

			return nil
		})
		if err != nil {
			return diag.FromErr(err)
		}

		d.SetId(getSyntheticsApiTestResponse.GetPublicId())

		return updateSyntheticsAPITestLocalState(d, &getSyntheticsApiTestResponse)
	} else if testType == datadogV1.SYNTHETICSTESTDETAILSTYPE_BROWSER {
		syntheticsTest := buildSyntheticsBrowserTestStruct(d)
		createdSyntheticsTest, httpResponse, err := datadogClientV1.SyntheticsApi.CreateSyntheticsBrowserTest(authV1, *syntheticsTest)
		if err != nil {
			// Note that Id won't be set, so no state will be saved.
			return utils.TranslateClientErrorDiag(err, httpResponse, "error creating synthetics browser test")
		}
		if err := utils.CheckForUnparsed(createdSyntheticsTest); err != nil {
			return diag.FromErr(err)
		}

		var getSyntheticsBrowserTestResponse datadogV1.SyntheticsBrowserTest
		var httpResponseGet *_nethttp.Response
		err = resource.RetryContext(ctx, d.Timeout(schema.TimeoutCreate), func() *resource.RetryError {
			getSyntheticsBrowserTestResponse, httpResponseGet, err = datadogClientV1.SyntheticsApi.GetBrowserTest(authV1, createdSyntheticsTest.GetPublicId())
			if err != nil {
				if httpResponseGet != nil && httpResponseGet.StatusCode == 404 {
					return resource.RetryableError(fmt.Errorf("synthetics browser test not created yet"))
				}

				return resource.NonRetryableError(err)
			}
			if err := utils.CheckForUnparsed(getSyntheticsBrowserTestResponse); err != nil {
				return resource.NonRetryableError(err)
			}

			return nil
		})
		if err != nil {
			return diag.FromErr(err)
		}

		d.SetId(getSyntheticsBrowserTestResponse.GetPublicId())

		return updateSyntheticsBrowserTestLocalState(d, &getSyntheticsBrowserTestResponse)
	}

	return diag.Errorf("unrecognized synthetics test type %v", testType)
}

func resourceDatadogSyntheticsTestRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	providerConf := meta.(*ProviderConfiguration)
	datadogClientV1 := providerConf.DatadogClientV1
	authV1 := providerConf.AuthV1

	var syntheticsTest datadogV1.SyntheticsTestDetails
	var syntheticsAPITest datadogV1.SyntheticsAPITest
	var syntheticsBrowserTest datadogV1.SyntheticsBrowserTest
	var err error
	var httpresp *_nethttp.Response

	// get the generic test to detect if it's an api or browser test
	syntheticsTest, httpresp, err = datadogClientV1.SyntheticsApi.GetTest(authV1, d.Id())
	if err != nil {
		if httpresp != nil && httpresp.StatusCode == 404 {
			// Delete the resource from the local state since it doesn't exist anymore in the actual state
			d.SetId("")
			return nil
		}
		return utils.TranslateClientErrorDiag(err, httpresp, "error getting synthetics test")
	}
	if err := utils.CheckForUnparsed(syntheticsTest); err != nil {
		return diag.FromErr(err)
	}

	if syntheticsTest.GetType() == datadogV1.SYNTHETICSTESTDETAILSTYPE_BROWSER {
		syntheticsBrowserTest, _, err = datadogClientV1.SyntheticsApi.GetBrowserTest(authV1, d.Id())
	} else {
		syntheticsAPITest, _, err = datadogClientV1.SyntheticsApi.GetAPITest(authV1, d.Id())
	}

	if err != nil {
		if httpresp != nil && httpresp.StatusCode == 404 {
			// Delete the resource from the local state since it doesn't exist anymore in the actual state
			d.SetId("")
			return nil
		}
		return utils.TranslateClientErrorDiag(err, httpresp, "error getting synthetics test")
	}

	if syntheticsTest.GetType() == datadogV1.SYNTHETICSTESTDETAILSTYPE_BROWSER {
		if err := utils.CheckForUnparsed(syntheticsBrowserTest); err != nil {
			return diag.FromErr(err)
		}
		return updateSyntheticsBrowserTestLocalState(d, &syntheticsBrowserTest)
	}

	if err := utils.CheckForUnparsed(syntheticsAPITest); err != nil {
		return diag.FromErr(err)
	}
	return updateSyntheticsAPITestLocalState(d, &syntheticsAPITest)
}

func resourceDatadogSyntheticsTestUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	providerConf := meta.(*ProviderConfiguration)
	datadogClientV1 := providerConf.DatadogClientV1
	authV1 := providerConf.AuthV1

	testType := getSyntheticsTestType(d)

	if testType == datadogV1.SYNTHETICSTESTDETAILSTYPE_API {
		syntheticsTest := buildSyntheticsAPITestStruct(d)
		updatedTest, httpResponse, err := datadogClientV1.SyntheticsApi.UpdateAPITest(authV1, d.Id(), *syntheticsTest)
		if err != nil {
			// If the Update callback returns with or without an error, the full state is saved.
			return utils.TranslateClientErrorDiag(err, httpResponse, "error updating synthetics API test")
		}
		if err := utils.CheckForUnparsed(updatedTest); err != nil {
			return diag.FromErr(err)
		}
		return updateSyntheticsAPITestLocalState(d, &updatedTest)
	} else if testType == datadogV1.SYNTHETICSTESTDETAILSTYPE_BROWSER {
		syntheticsTest := buildSyntheticsBrowserTestStruct(d)
		updatedTest, httpResponse, err := datadogClientV1.SyntheticsApi.UpdateBrowserTest(authV1, d.Id(), *syntheticsTest)
		if err != nil {
			// If the Update callback returns with or without an error, the full state is saved.
			return utils.TranslateClientErrorDiag(err, httpResponse, "error updating synthetics browser test")
		}
		if err := utils.CheckForUnparsed(updatedTest); err != nil {
			return diag.FromErr(err)
		}
		return updateSyntheticsBrowserTestLocalState(d, &updatedTest)
	}

	return diag.Errorf("unrecognized synthetics test type %v", testType)
}

func resourceDatadogSyntheticsTestDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	providerConf := meta.(*ProviderConfiguration)
	datadogClientV1 := providerConf.DatadogClientV1
	authV1 := providerConf.AuthV1

	syntheticsDeleteTestsPayload := datadogV1.SyntheticsDeleteTestsPayload{PublicIds: []string{d.Id()}}
	if _, httpResponse, err := datadogClientV1.SyntheticsApi.DeleteTests(authV1, syntheticsDeleteTestsPayload); err != nil {
		// The resource is assumed to still exist, and all prior state is preserved.
		return utils.TranslateClientErrorDiag(err, httpResponse, "error deleting synthetics test")
	}

	// The resource is assumed to be destroyed, and all state is removed.
	return nil
}

func isTargetOfTypeInt(assertionType datadogV1.SyntheticsAssertionType, assertionOperator datadogV1.SyntheticsAssertionOperator) bool {
	for _, intTargetAssertionType := range []datadogV1.SyntheticsAssertionType{
		datadogV1.SYNTHETICSASSERTIONTYPE_RESPONSE_TIME,
		datadogV1.SYNTHETICSASSERTIONTYPE_CERTIFICATE,
		datadogV1.SYNTHETICSASSERTIONTYPE_LATENCY,
		datadogV1.SYNTHETICSASSERTIONTYPE_PACKETS_RECEIVED,
		datadogV1.SYNTHETICSASSERTIONTYPE_NETWORK_HOP,
		datadogV1.SYNTHETICSASSERTIONTYPE_GRPC_HEALTHCHECK_STATUS,
	} {
		if assertionType == intTargetAssertionType {
			return true
		}
	}
	if assertionType == datadogV1.SYNTHETICSASSERTIONTYPE_STATUS_CODE &&
		(assertionOperator == datadogV1.SYNTHETICSASSERTIONOPERATOR_IS || assertionOperator == datadogV1.SYNTHETICSASSERTIONOPERATOR_IS_NOT) {
		return true
	}
	return false
}

func getSyntheticsTestType(d *schema.ResourceData) datadogV1.SyntheticsTestDetailsType {
	return datadogV1.SyntheticsTestDetailsType(d.Get("type").(string))
}

func buildSyntheticsAPITestStruct(d *schema.ResourceData) *datadogV1.SyntheticsAPITest {
	syntheticsTest := datadogV1.NewSyntheticsAPITestWithDefaults()
	syntheticsTest.SetName(d.Get("name").(string))

	if attr, ok := d.GetOk("subtype"); ok {
		syntheticsTest.SetSubtype(datadogV1.SyntheticsTestDetailsSubType(attr.(string)))
	} else {
		syntheticsTest.SetSubtype(datadogV1.SYNTHETICSTESTDETAILSSUBTYPE_HTTP)
	}

	k := utils.NewResourceDataKey(d, "")
	parts := "request_definition.0"
	k.Add(parts)

	request := datadogV1.SyntheticsTestRequest{}
	if attr, ok := k.GetOkWith("method"); ok {
		request.SetMethod(datadogV1.HTTPMethod(attr.(string)))
	}
	if attr, ok := k.GetOkWith("url"); ok {
		request.SetUrl(attr.(string))
	}
	if attr, ok := k.GetOkWith("body"); ok {
		request.SetBody(attr.(string))
	}
	if attr, ok := k.GetOkWith("timeout"); ok {
		request.SetTimeout(float64(attr.(int)))
	}
	if attr, ok := k.GetOkWith("host"); ok {
		request.SetHost(attr.(string))
	}
	if attr, ok := k.GetOkWith("port"); ok {
		request.SetPort(int64(attr.(int)))
	}
	if attr, ok := k.GetOkWith("dns_server"); ok {
		request.SetDnsServer(attr.(string))
	}
	if attr, ok := k.GetOkWith("dns_server_port"); ok {
		request.SetDnsServerPort(int32(attr.(int)))
	}
	if attr, ok := k.GetOkWith("no_saving_response_body"); ok {
		request.SetNoSavingResponseBody(attr.(bool))
	}
	if attr, ok := k.GetOkWith("number_of_packets"); ok {
		request.SetNumberOfPackets(int32(attr.(int)))
	}
	if attr, ok := k.GetOkWith("should_track_hops"); ok {
		request.SetShouldTrackHops(attr.(bool))
	}
	if attr, ok := k.GetOkWith("servername"); ok {
		request.SetServername(attr.(string))
	}
	if attr, ok := k.GetOkWith("message"); ok {
		request.SetMessage(attr.(string))
	}
	if attr, ok := k.GetOkWith("service"); ok {
		request.SetService(attr.(string))
	}
	k.Remove(parts)

	request = completeSyntheticsTestRequest(request, d.Get("request_headers").(map[string]interface{}), d.Get("request_query").(map[string]interface{}), d.Get("request_basicauth").([]interface{}), d.Get("request_client_certificate").([]interface{}), d.Get("request_proxy").([]interface{}))

	config := datadogV1.NewSyntheticsAPITestConfigWithDefaults()

	if syntheticsTest.GetSubtype() != "multi" {
		config.SetRequest(request)
	}

	config.Assertions = []datadogV1.SyntheticsAssertion{}
	if attr, ok := d.GetOk("assertion"); ok && attr != nil {
		assertions := buildAssertions(attr.([]interface{}))
		config.Assertions = assertions
	}

	configVariables := make([]datadogV1.SyntheticsConfigVariable, 0)

	if attr, ok := d.GetOk("config_variable"); ok && attr != nil {
		for _, v := range attr.([]interface{}) {
			variableMap := v.(map[string]interface{})
			variable := datadogV1.SyntheticsConfigVariable{}

			variable.SetName(variableMap["name"].(string))
			variable.SetType(datadogV1.SyntheticsConfigVariableType(variableMap["type"].(string)))

			if variable.GetType() != "global" {
				variable.SetPattern(variableMap["pattern"].(string))
				variable.SetExample(variableMap["example"].(string))
			}

			if variableMap["id"] != "" {
				variable.SetId(variableMap["id"].(string))
			}

			configVariables = append(configVariables, variable)
		}
	}

	config.SetConfigVariables(configVariables)

	if attr, ok := d.GetOk("api_step"); ok && syntheticsTest.GetSubtype() == "multi" {
		steps := []datadogV1.SyntheticsAPIStep{}

		for _, s := range attr.([]interface{}) {
			step := datadogV1.SyntheticsAPIStep{}
			stepMap := s.(map[string]interface{})

			step.SetName(stepMap["name"].(string))
			step.SetSubtype(datadogV1.SyntheticsAPIStepSubtype(stepMap["subtype"].(string)))

			extractedValues := buildExtractedValues(stepMap["extracted_value"].([]interface{}))
			step.SetExtractedValues(extractedValues)

			assertions := stepMap["assertion"].([]interface{})
			step.SetAssertions(buildAssertions(assertions))

			request := datadogV1.SyntheticsTestRequest{}
			requests := stepMap["request_definition"].([]interface{})
			if len(requests) > 0 && requests[0] != nil {
				requestMap := requests[0].(map[string]interface{})
				request.SetMethod(datadogV1.HTTPMethod(requestMap["method"].(string)))
				request.SetUrl(requestMap["url"].(string))
				request.SetBody(requestMap["body"].(string))
				request.SetTimeout(float64(requestMap["timeout"].(int)))
				request.SetAllowInsecure(requestMap["allow_insecure"].(bool))
				request.SetFollowRedirects(requestMap["follow_redirects"].(bool))
			}

			request = completeSyntheticsTestRequest(request, stepMap["request_headers"].(map[string]interface{}), stepMap["request_query"].(map[string]interface{}), stepMap["request_basicauth"].([]interface{}), stepMap["request_client_certificate"].([]interface{}), stepMap["request_proxy"].([]interface{}))

			step.SetRequest(request)

			step.SetAllowFailure(stepMap["allow_failure"].(bool))
			step.SetIsCritical(stepMap["is_critical"].(bool))

			optionsRetry := datadogV1.SyntheticsTestOptionsRetry{}
			retries := stepMap["retry"].([]interface{})
			if len(retries) > 0 && retries[0] != nil {
				retry := retries[0]

				if count, ok := retry.(map[string]interface{})["count"]; ok {
					optionsRetry.SetCount(int64(count.(int)))
				}
				if interval, ok := retry.(map[string]interface{})["interval"]; ok {
					optionsRetry.SetInterval(float64(interval.(int)))
				}
				step.SetRetry(optionsRetry)
			}

			steps = append(steps, step)
		}

		config.SetSteps(steps)
	}

	options := datadogV1.NewSyntheticsTestOptions()

	if attr, ok := d.GetOk("options_list"); ok && attr != nil {
		if attr, ok := d.GetOk("options_list.0.tick_every"); ok {
			options.SetTickEvery(int64(attr.(int)))
		}
		if attr, ok := d.GetOk("options_list.0.accept_self_signed"); ok {
			options.SetAcceptSelfSigned(attr.(bool))
		}
		if attr, ok := d.GetOk("options_list.0.check_certificate_revocation"); ok {
			options.SetCheckCertificateRevocation(attr.(bool))
		}
		if attr, ok := d.GetOk("options_list.0.min_location_failed"); ok {
			options.SetMinLocationFailed(int64(attr.(int)))
		}
		if attr, ok := d.GetOk("options_list.0.min_failure_duration"); ok {
			options.SetMinFailureDuration(int64(attr.(int)))
		}
		if attr, ok := d.GetOk("options_list.0.follow_redirects"); ok {
			options.SetFollowRedirects(attr.(bool))
		}
		if attr, ok := d.GetOk("options_list.0.allow_insecure"); ok {
			options.SetAllowInsecure(attr.(bool))
		}

		if retryRaw, ok := d.GetOk("options_list.0.retry"); ok {
			optionsRetry := datadogV1.SyntheticsTestOptionsRetry{}
			retry := retryRaw.([]interface{})[0]

			if count, ok := retry.(map[string]interface{})["count"]; ok {
				optionsRetry.SetCount(int64(count.(int)))
			}
			if interval, ok := retry.(map[string]interface{})["interval"]; ok {
				optionsRetry.SetInterval(float64(interval.(int)))
			}

			options.SetRetry(optionsRetry)
		}

		if monitorOptionsRaw, ok := d.GetOk("options_list.0.monitor_options"); ok {
			monitorOptions := monitorOptionsRaw.([]interface{})[0]
			optionsMonitorOptions := datadogV1.SyntheticsTestOptionsMonitorOptions{}

			if renotifyInterval, ok := monitorOptions.(map[string]interface{})["renotify_interval"]; ok {
				optionsMonitorOptions.SetRenotifyInterval(int64(renotifyInterval.(int)))
			}

			options.SetMonitorOptions(optionsMonitorOptions)
		}

		if monitorName, ok := d.GetOk("options_list.0.monitor_name"); ok {
			options.SetMonitorName(monitorName.(string))
		}

		if monitorPriority, ok := d.GetOk("options_list.0.monitor_priority"); ok {
			options.SetMonitorPriority(int32(monitorPriority.(int)))
		}

		if restricted_roles, ok := d.GetOk("options_list.0.restricted_roles"); ok {
			roles := []string{}
			for _, role := range restricted_roles.(*schema.Set).List() {
				roles = append(roles, role.(string))
			}
			options.SetRestrictedRoles(roles)
		}

		if ciRaw, ok := d.GetOk("options_list.0.ci"); ok {
			ci := ciRaw.([]interface{})[0]
			testCiOptions := ci.(map[string]interface{})

			ciOptions := datadogV1.SyntheticsTestCiOptions{}
			ciOptions.SetExecutionRule(datadogV1.SyntheticsTestExecutionRule(testCiOptions["execution_rule"].(string)))

			options.SetCi(ciOptions)
		}
	}

	if attr, ok := d.GetOk("device_ids"); ok {
		var deviceIds []datadogV1.SyntheticsDeviceID
		for _, s := range attr.([]interface{}) {
			deviceIds = append(deviceIds, datadogV1.SyntheticsDeviceID(s.(string)))
		}
		options.DeviceIds = deviceIds
	}

	syntheticsTest.SetConfig(*config)
	syntheticsTest.SetOptions(*options)
	syntheticsTest.SetMessage(d.Get("message").(string))
	syntheticsTest.SetStatus(datadogV1.SyntheticsTestPauseStatus(d.Get("status").(string)))

	if attr, ok := d.GetOk("locations"); ok {
		var locations []string
		for _, s := range attr.(*schema.Set).List() {
			locations = append(locations, s.(string))
		}
		syntheticsTest.SetLocations(locations)
	}

	tags := make([]string, 0)
	if attr, ok := d.GetOk("tags"); ok {
		for _, s := range attr.([]interface{}) {
			tags = append(tags, s.(string))
		}
	}
	syntheticsTest.SetTags(tags)

	return syntheticsTest
}

func completeSyntheticsTestRequest(request datadogV1.SyntheticsTestRequest, requestHeaders map[string]interface{}, requestQuery map[string]interface{}, basicAuth []interface{}, requestClientCertificates []interface{}, requestProxy []interface{}) datadogV1.SyntheticsTestRequest {
	if len(requestHeaders) > 0 {
		headers := make(map[string]string, len(requestHeaders))

		for k, v := range requestHeaders {
			headers[k] = v.(string)
		}

		request.SetHeaders(headers)
	}

	if len(requestQuery) > 0 {
		request.SetQuery(requestQuery)
	}

	if len(basicAuth) > 0 {
		if requestBasicAuth, ok := basicAuth[0].(map[string]interface{}); ok {
			if requestBasicAuth["type"] == "web" && requestBasicAuth["username"] != "" && requestBasicAuth["password"] != "" {
				basicAuth := datadogV1.NewSyntheticsBasicAuthWebWithDefaults()
				basicAuth.SetPassword(requestBasicAuth["password"].(string))
				basicAuth.SetUsername(requestBasicAuth["username"].(string))
				request.SetBasicAuth(datadogV1.SyntheticsBasicAuthWebAsSyntheticsBasicAuth(basicAuth))
			}

			if requestBasicAuth["type"] == "sigv4" && requestBasicAuth["access_key"] != "" && requestBasicAuth["secret_key"] != "" {
				basicAuth := datadogV1.NewSyntheticsBasicAuthSigv4(requestBasicAuth["access_key"].(string), requestBasicAuth["secret_key"].(string), datadogV1.SYNTHETICSBASICAUTHSIGV4TYPE_SIGV4)

				basicAuth.SetRegion(requestBasicAuth["region"].(string))
				basicAuth.SetServiceName(requestBasicAuth["service_name"].(string))
				basicAuth.SetSessionToken(requestBasicAuth["session_token"].(string))

				request.SetBasicAuth(datadogV1.SyntheticsBasicAuthSigv4AsSyntheticsBasicAuth(basicAuth))
			}

			if requestBasicAuth["type"] == "ntlm" {
				basicAuth := datadogV1.NewSyntheticsBasicAuthNTLM(datadogV1.SYNTHETICSBASICAUTHNTLMTYPE_NTLM)

				basicAuth.SetUsername(requestBasicAuth["username"].(string))
				basicAuth.SetPassword(requestBasicAuth["password"].(string))
				basicAuth.SetDomain(requestBasicAuth["domain"].(string))
				basicAuth.SetWorkstation(requestBasicAuth["workstation"].(string))

				request.SetBasicAuth(datadogV1.SyntheticsBasicAuthNTLMAsSyntheticsBasicAuth(basicAuth))
			}
		}
	}

	if len(requestClientCertificates) > 0 {
		cert := datadogV1.SyntheticsTestRequestCertificateItem{}
		key := datadogV1.SyntheticsTestRequestCertificateItem{}
		clientCertificate := requestClientCertificates[0].(map[string]interface{})
		clientCerts := clientCertificate["cert"].([]interface{})
		clientKeys := clientCertificate["key"].([]interface{})

		clientCert := clientCerts[0].(map[string]interface{})
		clientKey := clientKeys[0].(map[string]interface{})

		if clientCert["content"] != "" {
			// only set the certificate content if it is not an already hashed string
			// this is needed for the update function that receives the data from the state
			// and not from the config. So we get a hash of the certificate and not it's real
			// value.
			if isHash := isCertHash(clientCert["content"].(string)); !isHash {
				cert.SetContent(clientCert["content"].(string))
			}
		}
		if clientCert["filename"] != "" {
			cert.SetFilename(clientCert["filename"].(string))
		}

		if clientKey["content"] != "" {
			// only set the key content if it is not an already hashed string
			if isHash := isCertHash(clientKey["content"].(string)); !isHash {
				key.SetContent(clientKey["content"].(string))
			}
		}
		if clientKey["filename"] != "" {
			key.SetFilename(clientKey["filename"].(string))
		}

		requestClientCertificate := datadogV1.SyntheticsTestRequestCertificate{
			Cert: &cert,
			Key:  &key,
		}

		request.SetCertificate(requestClientCertificate)
	}

	if len(requestProxy) > 0 {
		if proxy, ok := requestProxy[0].(map[string]interface{}); ok {
			testRequestProxy := datadogV1.SyntheticsTestRequestProxy{}
			testRequestProxy.SetUrl(proxy["url"].(string))

			proxyHeaders := make(map[string]string, len(proxy["headers"].(map[string]interface{})))

			for k, v := range proxy["headers"].(map[string]interface{}) {
				proxyHeaders[k] = v.(string)
			}

			testRequestProxy.SetHeaders(proxyHeaders)

			request.SetProxy(testRequestProxy)
		}
	}

	return request
}

func buildAssertions(attr []interface{}) []datadogV1.SyntheticsAssertion {
	assertions := make([]datadogV1.SyntheticsAssertion, 0)

	for _, assertion := range attr {
		assertionMap := assertion.(map[string]interface{})
		if v, ok := assertionMap["type"]; ok {
			assertionType := v.(string)
			if v, ok := assertionMap["operator"]; ok {
				assertionOperator := v.(string)
				if assertionOperator == string(datadogV1.SYNTHETICSASSERTIONJSONPATHOPERATOR_VALIDATES_JSON_PATH) {
					assertionJSONPathTarget := datadogV1.NewSyntheticsAssertionJSONPathTarget(datadogV1.SyntheticsAssertionJSONPathOperator(assertionOperator), datadogV1.SyntheticsAssertionType(assertionType))
					if v, ok := assertionMap["property"].(string); ok && len(v) > 0 {
						assertionJSONPathTarget.SetProperty(v)
					}
					if v, ok := assertionMap["targetjsonpath"].([]interface{}); ok && len(v) > 0 {
						subTarget := datadogV1.NewSyntheticsAssertionJSONPathTargetTarget()
						targetMap := v[0].(map[string]interface{})
						if v, ok := targetMap["jsonpath"]; ok {
							subTarget.SetJsonPath(v.(string))
						}
						operator, ok := targetMap["operator"]
						if ok {
							subTarget.SetOperator(operator.(string))
						}
						if v, ok := targetMap["targetvalue"]; ok {
							switch datadogV1.SyntheticsAssertionOperator(operator.(string)) {
							case
								datadogV1.SYNTHETICSASSERTIONOPERATOR_LESS_THAN,
								datadogV1.SYNTHETICSASSERTIONOPERATOR_MORE_THAN:
								if match, _ := regexp.MatchString("{{\\s*([^{}]*?)\\s*}}", v.(string)); match {
									subTarget.SetTargetValue(v)
								} else {
									setFloatTargetValue(subTarget, v.(string))
								}
							default:
								subTarget.SetTargetValue(v)
							}
						}
						assertionJSONPathTarget.SetTarget(*subTarget)
					}
					if _, ok := assertionMap["target"]; ok {
						log.Printf("[WARN] target shouldn't be specified for validateJSONPath operator, only targetjsonpath")
					}
					assertions = append(assertions, datadogV1.SyntheticsAssertionJSONPathTargetAsSyntheticsAssertion(assertionJSONPathTarget))
				} else {
					assertionTarget := datadogV1.NewSyntheticsAssertionTargetWithDefaults()
					assertionTarget.SetOperator(datadogV1.SyntheticsAssertionOperator(assertionOperator))
					assertionTarget.SetType(datadogV1.SyntheticsAssertionType(assertionType))
					if v, ok := assertionMap["property"].(string); ok && len(v) > 0 {
						assertionTarget.SetProperty(v)
					}
					if v, ok := assertionMap["target"]; ok {
						if isTargetOfTypeInt(assertionTarget.GetType(), assertionTarget.GetOperator()) {
							assertionTargetInt, _ := strconv.Atoi(v.(string))
							assertionTarget.SetTarget(assertionTargetInt)
						} else if assertionTarget.GetType() == datadogV1.SYNTHETICSASSERTIONTYPE_PACKET_LOSS_PERCENTAGE {
							assertionTargetFloat, _ := strconv.ParseFloat(v.(string), 64)
							assertionTarget.SetTarget(assertionTargetFloat)
						} else {
							assertionTarget.SetTarget(v.(string))
						}
					}
					if v, ok := assertionMap["targetjsonpath"].([]interface{}); ok && len(v) > 0 {
						log.Printf("[WARN] targetjsonpath shouldn't be specified for non-validateJSONPath operator, only target")
					}
					assertions = append(assertions, datadogV1.SyntheticsAssertionTargetAsSyntheticsAssertion(assertionTarget))
				}
			}
		}
	}

	return assertions
}

func buildSyntheticsBrowserTestStruct(d *schema.ResourceData) *datadogV1.SyntheticsBrowserTest {
	request := datadogV1.SyntheticsTestRequest{}
	k := utils.NewResourceDataKey(d, "")
	parts := "request_definition.0"
	k.Add(parts)
	if attr, ok := k.GetOkWith("method"); ok {
		request.SetMethod(datadogV1.HTTPMethod(attr.(string)))
	}
	if attr, ok := k.GetOkWith("url"); ok {
		request.SetUrl(attr.(string))
	}
	if attr, ok := k.GetOkWith("body"); ok {
		request.SetBody(attr.(string))
	}
	if attr, ok := k.GetOkWith("timeout"); ok {
		request.SetTimeout(float64(attr.(int)))
	}
	k.Remove(parts)
	if attr, ok := d.GetOk("request_query"); ok {
		query := attr.(map[string]interface{})
		if len(query) > 0 {
			request.SetQuery(query)
		}
	}

	if username, ok := d.GetOk("request_basicauth.0.username"); ok {
		if password, ok := d.GetOk("request_basicauth.0.password"); ok {
			basicAuth := datadogV1.NewSyntheticsBasicAuthWebWithDefaults()
			basicAuth.SetPassword(password.(string))
			basicAuth.SetUsername(username.(string))
			request.SetBasicAuth(datadogV1.SyntheticsBasicAuthWebAsSyntheticsBasicAuth(basicAuth))
		}
	}

	if attr, ok := d.GetOk("request_headers"); ok {
		headers := attr.(map[string]interface{})
		if len(headers) > 0 {
			request.SetHeaders(make(map[string]string))
		}
		for k, v := range headers {
			request.GetHeaders()[k] = v.(string)
		}
	}

	if _, ok := d.GetOk("request_client_certificate"); ok {
		cert := datadogV1.SyntheticsTestRequestCertificateItem{}
		key := datadogV1.SyntheticsTestRequestCertificateItem{}

		if attr, ok := d.GetOk("request_client_certificate.0.cert.0.filename"); ok {
			cert.SetFilename(attr.(string))
		}
		if attr, ok := d.GetOk("request_client_certificate.0.cert.0.content"); ok {
			cert.SetContent(attr.(string))
		}

		if attr, ok := d.GetOk("request_client_certificate.0.key.0.filename"); ok {
			key.SetFilename(attr.(string))
		}
		if attr, ok := d.GetOk("request_client_certificate.0.key.0.content"); ok {
			key.SetContent(attr.(string))
		}

		clientCertificate := datadogV1.SyntheticsTestRequestCertificate{
			Cert: &cert,
			Key:  &key,
		}

		request.SetCertificate(clientCertificate)
	}

	if _, ok := d.GetOk("request_proxy"); ok {
		requestProxy := datadogV1.SyntheticsTestRequestProxy{}

		if url, ok := d.GetOk("request_proxy.0.url"); ok {
			requestProxy.SetUrl(url.(string))

			if headers, ok := d.GetOk("request_proxy.0.headers"); ok {
				proxyHeaders := make(map[string]string, len(headers.(map[string]interface{})))

				for k, v := range headers.(map[string]interface{}) {
					proxyHeaders[k] = v.(string)
				}

				requestProxy.SetHeaders(proxyHeaders)
			}

			request.SetProxy(requestProxy)
		}
	}

	config := datadogV1.SyntheticsBrowserTestConfig{}
	config.SetAssertions([]datadogV1.SyntheticsAssertion{})
	config.SetRequest(request)
	config.SetVariables([]datadogV1.SyntheticsBrowserVariable{})

	var browserVariables []interface{}

	if attr, ok := d.GetOk("browser_variable"); ok && attr != nil {
		browserVariables = attr.([]interface{})
	}

	for _, variable := range browserVariables {
		variableMap := variable.(map[string]interface{})
		if v, ok := variableMap["type"]; ok {
			variableType := datadogV1.SyntheticsBrowserVariableType(v.(string))
			if v, ok := variableMap["name"]; ok {
				variableName := v.(string)
				newVariable := datadogV1.NewSyntheticsBrowserVariable(variableName, variableType)
				if v, ok := variableMap["example"]; ok && v.(string) != "" {
					newVariable.SetExample(v.(string))
				}
				if v, ok := variableMap["id"]; ok && v.(string) != "" {
					newVariable.SetId(v.(string))
				}
				if v, ok := variableMap["pattern"]; ok && v.(string) != "" {
					newVariable.SetPattern(v.(string))
				}
				config.SetVariables(append(config.GetVariables(), *newVariable))
			}
		}
	}

	configVariables := make([]datadogV1.SyntheticsConfigVariable, 0)

	if attr, ok := d.GetOk("config_variable"); ok && attr != nil {
		for _, v := range attr.([]interface{}) {
			variableMap := v.(map[string]interface{})
			variable := datadogV1.SyntheticsConfigVariable{}

			variable.SetName(variableMap["name"].(string))
			variable.SetType(datadogV1.SyntheticsConfigVariableType(variableMap["type"].(string)))

			if variable.GetType() != "global" {
				variable.SetPattern(variableMap["pattern"].(string))
				variable.SetExample(variableMap["example"].(string))
			}

			if variableMap["id"] != "" {
				variable.SetId(variableMap["id"].(string))
			}

			configVariables = append(configVariables, variable)
		}
	}

	config.SetConfigVariables(configVariables)

	options := datadogV1.NewSyntheticsTestOptions()

	if attr, ok := d.GetOk("options_list"); ok && attr != nil {
		if attr, ok := d.GetOk("options_list.0.tick_every"); ok {
			options.SetTickEvery(int64(attr.(int)))
		}
		if attr, ok := d.GetOk("options_list.0.accept_self_signed"); ok {
			options.SetAcceptSelfSigned(attr.(bool))
		}
		if attr, ok := d.GetOk("options_list.0.min_location_failed"); ok {
			options.SetMinLocationFailed(int64(attr.(int)))
		}
		if attr, ok := d.GetOk("options_list.0.min_failure_duration"); ok {
			options.SetMinFailureDuration(int64(attr.(int)))
		}
		if attr, ok := d.GetOk("options_list.0.follow_redirects"); ok {
			options.SetFollowRedirects(attr.(bool))
		}
		if attr, ok := d.GetOk("options_list.0.allow_insecure"); ok {
			options.SetAllowInsecure(attr.(bool))
		}

		if retryRaw, ok := d.GetOk("options_list.0.retry"); ok {
			optionsRetry := datadogV1.SyntheticsTestOptionsRetry{}
			retry := retryRaw.([]interface{})[0]

			if count, ok := retry.(map[string]interface{})["count"]; ok {
				optionsRetry.SetCount(int64(count.(int)))
			}
			if interval, ok := retry.(map[string]interface{})["interval"]; ok {
				optionsRetry.SetInterval(float64(interval.(int)))
			}

			options.SetRetry(optionsRetry)
		}

		if monitorOptionsRaw, ok := d.GetOk("options_list.0.monitor_options"); ok {
			monitorOptions := monitorOptionsRaw.([]interface{})[0]
			optionsMonitorOptions := datadogV1.SyntheticsTestOptionsMonitorOptions{}

			if renotifyInterval, ok := monitorOptions.(map[string]interface{})["renotify_interval"]; ok {
				optionsMonitorOptions.SetRenotifyInterval(int64(renotifyInterval.(int)))
			}

			options.SetMonitorOptions(optionsMonitorOptions)
		}

		if attr, ok := d.GetOk("options_list.0.no_screenshot"); ok {
			options.SetNoScreenshot(attr.(bool))
		}

		if monitorName, ok := d.GetOk("options_list.0.monitor_name"); ok {
			options.SetMonitorName(monitorName.(string))
		}

		if monitorPriority, ok := d.GetOk("options_list.0.monitor_priority"); ok {
			options.SetMonitorPriority(int32(monitorPriority.(int)))
		}

		if restricted_roles, ok := d.GetOk("options_list.0.restricted_roles"); ok {
			roles := []string{}
			for _, role := range restricted_roles.(*schema.Set).List() {
				roles = append(roles, role.(string))
			}
			options.SetRestrictedRoles(roles)
		}

		if ciRaw, ok := d.GetOk("options_list.0.ci"); ok {
			ci := ciRaw.([]interface{})[0]
			testCiOptions := ci.(map[string]interface{})

			ciOptions := datadogV1.SyntheticsTestCiOptions{}
			ciOptions.SetExecutionRule(datadogV1.SyntheticsTestExecutionRule(testCiOptions["execution_rule"].(string)))

			options.SetCi(ciOptions)
		}

		if rum_settings, ok := d.GetOk("options_list.0.rum_settings.0"); ok {
			settings := rum_settings.(map[string]interface{})
			isEnabled := settings["is_enabled"]

			rumSettings := datadogV1.SyntheticsBrowserTestRumSettings{}

			if isEnabled == true {
				rumSettings.SetIsEnabled(true)

				if applicationId, ok := settings["application_id"]; ok {
					rumSettings.SetApplicationId(applicationId.(string))
				}

				if clientTokenId, ok := settings["client_token_id"]; ok {
					rumSettings.SetClientTokenId(int64(clientTokenId.(int)))
				}
			} else {
				rumSettings.SetIsEnabled(false)
			}

			options.SetRumSettings(rumSettings)
		}
	}

	if attr, ok := d.GetOk("device_ids"); ok {
		var deviceIds []datadogV1.SyntheticsDeviceID
		for _, s := range attr.([]interface{}) {
			deviceIds = append(deviceIds, datadogV1.SyntheticsDeviceID(s.(string)))
		}
		options.DeviceIds = deviceIds
	}

	if attr, ok := d.GetOk("set_cookie"); ok {
		config.SetSetCookie(attr.(string))
	}

	syntheticsTest := datadogV1.NewSyntheticsBrowserTestWithDefaults()
	syntheticsTest.SetMessage(d.Get("message").(string))
	syntheticsTest.SetName(d.Get("name").(string))
	syntheticsTest.SetConfig(config)
	syntheticsTest.SetOptions(*options)
	syntheticsTest.SetStatus(datadogV1.SyntheticsTestPauseStatus(d.Get("status").(string)))

	if attr, ok := d.GetOk("locations"); ok {
		var locations []string
		for _, s := range attr.(*schema.Set).List() {
			locations = append(locations, s.(string))
		}
		syntheticsTest.SetLocations(locations)
	}

	tags := make([]string, 0)
	if attr, ok := d.GetOk("tags"); ok {
		for _, s := range attr.([]interface{}) {
			tags = append(tags, s.(string))
		}
	}
	syntheticsTest.SetTags(tags)

	if attr, ok := d.GetOk("browser_step"); ok {
		steps := []datadogV1.SyntheticsStep{}

		for _, s := range attr.([]interface{}) {
			step := datadogV1.SyntheticsStep{}
			stepMap := s.(map[string]interface{})

			step.SetName(stepMap["name"].(string))
			step.SetType(datadogV1.SyntheticsStepType(stepMap["type"].(string)))
			step.SetAllowFailure(stepMap["allow_failure"].(bool))
			step.SetIsCritical(stepMap["is_critical"].(bool))
			step.SetTimeout(int64(stepMap["timeout"].(int)))

			params := make(map[string]interface{})
			stepParams := stepMap["params"].([]interface{})[0]
			stepTypeParams := getParamsKeysForStepType(step.GetType())

			for _, key := range stepTypeParams {
				if stepMap, ok := stepParams.(map[string]interface{}); ok && stepMap[key] != "" {
					convertedValue := convertStepParamsValueForConfig(step.GetType(), key, stepMap[key])
					params[convertStepParamsKey(key)] = convertedValue
				}
			}

			if stepParamsMap, ok := stepParams.(map[string]interface{}); ok && stepParamsMap["element_user_locator"] != "" {
				userLocatorsParams := stepParamsMap["element_user_locator"].([]interface{})

				if len(userLocatorsParams) != 0 {
					userLocatorParams := userLocatorsParams[0].(map[string]interface{})
					values := userLocatorParams["value"].([]interface{})
					userLocator := map[string]interface{}{
						"failTestOnCannotLocate": userLocatorParams["fail_test_on_cannot_locate"],
						"values":                 []map[string]interface{}{values[0].(map[string]interface{})},
					}

					stepElement := make(map[string]interface{})
					if stepParamsElement, ok := stepParamsMap["element"]; ok {
						utils.GetMetadataFromJSON([]byte(stepParamsElement.(string)), &stepElement)
					}
					stepElement["userLocator"] = userLocator
					params["element"] = stepElement
				}
			}

			step.SetParams(params)

			steps = append(steps, step)
		}

		syntheticsTest.SetSteps(steps)
	}

	return syntheticsTest
}

func buildLocalRequest(request datadogV1.SyntheticsTestRequest) map[string]interface{} {
	localRequest := make(map[string]interface{})
	if request.HasBody() {
		localRequest["body"] = request.GetBody()
	}
	if request.HasMethod() {
		localRequest["method"] = convertToString(request.GetMethod())
	}
	if request.HasTimeout() {
		localRequest["timeout"] = request.GetTimeout()
	}
	if request.HasUrl() {
		localRequest["url"] = request.GetUrl()
	}
	if request.HasHost() {
		localRequest["host"] = request.GetHost()
	}
	if request.HasPort() {
		localRequest["port"] = request.GetPort()
	}
	if request.HasDnsServer() {
		localRequest["dns_server"] = convertToString(request.GetDnsServer())
	}
	if request.HasDnsServerPort() {
		localRequest["dns_server_port"] = request.GetDnsServerPort()
	}
	if request.HasNoSavingResponseBody() {
		localRequest["no_saving_response_body"] = request.GetNoSavingResponseBody()
	}
	if request.HasNumberOfPackets() {
		localRequest["number_of_packets"] = request.GetNumberOfPackets()
	}
	if request.HasShouldTrackHops() {
		localRequest["should_track_hops"] = request.GetShouldTrackHops()
	}
	if request.HasServername() {
		localRequest["servername"] = request.GetServername()
	}
	if request.HasMessage() {
		localRequest["message"] = request.GetMessage()
	}
	if request.HasService() {
		localRequest["service"] = request.GetService()
	}

	return localRequest
}

func buildLocalAssertions(actualAssertions []datadogV1.SyntheticsAssertion) (localAssertions []map[string]interface{}, err error) {
	localAssertions = make([]map[string]interface{}, len(actualAssertions))
	for i, assertion := range actualAssertions {
		localAssertion := make(map[string]interface{})
		if assertion.SyntheticsAssertionTarget != nil {
			assertionTarget := assertion.SyntheticsAssertionTarget
			if v, ok := assertionTarget.GetOperatorOk(); ok {
				localAssertion["operator"] = string(*v)
			}
			if assertionTarget.HasProperty() {
				localAssertion["property"] = assertionTarget.GetProperty()
			}
			if target := assertionTarget.GetTarget(); target != nil {
				localAssertion["target"] = convertToString(target)
			}
			if v, ok := assertionTarget.GetTypeOk(); ok {
				localAssertion["type"] = string(*v)
			}
		} else if assertion.SyntheticsAssertionJSONPathTarget != nil {
			assertionTarget := assertion.SyntheticsAssertionJSONPathTarget
			if v, ok := assertionTarget.GetOperatorOk(); ok {
				localAssertion["operator"] = string(*v)
			}
			if assertionTarget.HasProperty() {
				localAssertion["property"] = assertionTarget.GetProperty()
			}
			if target, ok := assertionTarget.GetTargetOk(); ok {
				localTarget := make(map[string]string)
				if v, ok := target.GetJsonPathOk(); ok {
					localTarget["jsonpath"] = string(*v)
				}
				if v, ok := target.GetOperatorOk(); ok {
					localTarget["operator"] = string(*v)
				}
				if v, ok := target.GetTargetValueOk(); ok {
					val := (*v).(interface{})
					if vAsString, ok := val.(string); ok {
						localTarget["targetvalue"] = vAsString
					} else if vAsFloat, ok := val.(float64); ok {
						localTarget["targetvalue"] = strconv.FormatFloat(vAsFloat, 'f', -1, 64)
					} else {
						return localAssertions, fmt.Errorf("unrecognized targetvalue type %v", v)
					}
				}
				localAssertion["targetjsonpath"] = []map[string]string{localTarget}
			}
			if v, ok := assertionTarget.GetTypeOk(); ok {
				localAssertion["type"] = string(*v)
			}
		}
		localAssertions[i] = localAssertion
	}

	return localAssertions, nil
}

func buildLocalBasicAuth(basicAuth *datadogV1.SyntheticsBasicAuth) map[string]string {
	localAuth := make(map[string]string)

	if basicAuth.SyntheticsBasicAuthWeb != nil {
		basicAuthWeb := basicAuth.SyntheticsBasicAuthWeb

		localAuth["username"] = basicAuthWeb.Username
		localAuth["password"] = basicAuthWeb.Password
		localAuth["type"] = "web"
	}

	if basicAuth.SyntheticsBasicAuthSigv4 != nil {
		basicAuthSigv4 := basicAuth.SyntheticsBasicAuthSigv4
		localAuth["access_key"] = basicAuthSigv4.AccessKey
		localAuth["secret_key"] = basicAuthSigv4.SecretKey
		localAuth["region"] = *basicAuthSigv4.Region
		localAuth["session_token"] = *basicAuthSigv4.SessionToken
		localAuth["service_name"] = *basicAuthSigv4.ServiceName
		localAuth["type"] = "sigv4"
	}

	if basicAuth.SyntheticsBasicAuthNTLM != nil {
		basicAuthNtlm := basicAuth.SyntheticsBasicAuthNTLM
		localAuth["username"] = *basicAuthNtlm.Username
		localAuth["password"] = *basicAuthNtlm.Password
		localAuth["domain"] = *basicAuthNtlm.Domain
		localAuth["workstation"] = *basicAuthNtlm.Workstation
		localAuth["type"] = "ntlm"
	}

	return localAuth
}

func buildExtractedValues(stepExtractedValues []interface{}) []datadogV1.SyntheticsParsingOptions {
	values := make([]datadogV1.SyntheticsParsingOptions, len(stepExtractedValues))

	for i, extractedValue := range stepExtractedValues {
		extractedValueMap := extractedValue.(map[string]interface{})
		value := datadogV1.SyntheticsParsingOptions{}

		value.SetName(extractedValueMap["name"].(string))
		value.SetField(extractedValueMap["field"].(string))
		value.SetType(datadogV1.SyntheticsGlobalVariableParseTestOptionsType(extractedValueMap["type"].(string)))

		valueParsers := extractedValueMap["parser"].([]interface{})
		valueParser := valueParsers[0].(map[string]interface{})

		parser := datadogV1.SyntheticsVariableParser{}
		parser.SetType(datadogV1.SyntheticsGlobalVariableParserType(valueParser["type"].(string)))
		parser.SetValue(valueParser["value"].(string))

		value.SetParser(parser)

		values[i] = value
	}

	return values
}

func buildLocalExtractedValues(extractedValues []datadogV1.SyntheticsParsingOptions) []map[string]interface{} {
	localExtractedValues := make([]map[string]interface{}, len(extractedValues))

	for i, extractedValue := range extractedValues {
		localExtractedValue := make(map[string]interface{})
		localExtractedValue["name"] = extractedValue.GetName()
		localExtractedValue["type"] = string(extractedValue.GetType())
		localExtractedValue["field"] = extractedValue.GetField()

		parser := extractedValue.GetParser()
		localParser := make(map[string]interface{})
		localParser["type"] = string(parser.GetType())
		localParser["value"] = parser.GetValue()
		localExtractedValue["parser"] = []map[string]interface{}{localParser}

		localExtractedValues[i] = localExtractedValue
	}

	return localExtractedValues
}

func updateSyntheticsBrowserTestLocalState(d *schema.ResourceData, syntheticsTest *datadogV1.SyntheticsBrowserTest) diag.Diagnostics {
	if err := d.Set("type", syntheticsTest.GetType()); err != nil {
		return diag.FromErr(err)
	}

	config := syntheticsTest.GetConfig()
	actualRequest := config.GetRequest()
	localRequest := buildLocalRequest(actualRequest)

	if config.HasSetCookie() {
		if err := d.Set("set_cookie", config.GetSetCookie()); err != nil {
			return diag.FromErr(err)
		}
	}
	if err := d.Set("request_definition", []map[string]interface{}{localRequest}); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("request_headers", actualRequest.Headers); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("request_query", actualRequest.GetQuery()); err != nil {
		return diag.FromErr(err)
	}

	if basicAuth, ok := actualRequest.GetBasicAuthOk(); ok && basicAuth.SyntheticsBasicAuthWeb != nil {
		localAuth := buildLocalBasicAuth(basicAuth)

		if err := d.Set("request_basicauth", []map[string]string{localAuth}); err != nil {
			return diag.FromErr(err)
		}
	}

	if clientCertificate, ok := actualRequest.GetCertificateOk(); ok {
		localCertificate := make(map[string][]map[string]string)
		localCertificate["cert"] = make([]map[string]string, 1)
		localCertificate["cert"][0] = make(map[string]string)
		localCertificate["key"] = make([]map[string]string, 1)
		localCertificate["key"][0] = make(map[string]string)

		cert := clientCertificate.GetCert()
		localCertificate["cert"][0]["filename"] = cert.GetFilename()

		key := clientCertificate.GetKey()
		localCertificate["key"][0]["filename"] = key.GetFilename()

		// the content of client certificate is write-only so it will not be returned by the API.
		// To avoid useless diff but also prevent storing the value in clear in the state
		// we store a hash of the value.
		if configCertificateContent, ok := d.GetOk("request_client_certificate.0.cert.0.content"); ok {
			localCertificate["cert"][0]["content"] = getCertificateStateValue(configCertificateContent.(string))
		}
		if configKeyContent, ok := d.GetOk("request_client_certificate.0.key.0.content"); ok {
			localCertificate["key"][0]["content"] = getCertificateStateValue(configKeyContent.(string))
		}

		if err := d.Set("request_client_certificate", []map[string][]map[string]string{localCertificate}); err != nil {
			return diag.FromErr(err)
		}
	}

	if proxy, ok := actualRequest.GetProxyOk(); ok {
		localProxy := make(map[string]interface{})
		localProxy["url"] = proxy.GetUrl()
		localProxy["headers"] = proxy.GetHeaders()

		d.Set("request_proxy", []map[string]interface{}{localProxy})
	}

	// assertions are required but not used for browser tests
	localAssertions := make([]map[string]interface{}, 0)

	if err := d.Set("assertion", localAssertions); err != nil {
		return diag.FromErr(err)
	}

	configVariables := config.GetConfigVariables()
	localConfigVariables := make([]map[string]interface{}, len(configVariables))
	for i, configVariable := range configVariables {
		localVariable := make(map[string]interface{})
		if v, ok := configVariable.GetTypeOk(); ok {
			localVariable["type"] = *v
		}
		if v, ok := configVariable.GetNameOk(); ok {
			localVariable["name"] = *v
		}

		if configVariable.GetType() != "global" {
			if v, ok := configVariable.GetExampleOk(); ok {
				localVariable["example"] = *v
			}
			if v, ok := configVariable.GetPatternOk(); ok {
				localVariable["pattern"] = *v
			}
		}
		if v, ok := configVariable.GetIdOk(); ok {
			localVariable["id"] = *v
		}
		localConfigVariables[i] = localVariable
	}

	if err := d.Set("config_variable", localConfigVariables); err != nil {
		return diag.FromErr(err)
	}

	actualVariables := config.GetVariables()
	localBrowserVariables := make([]map[string]interface{}, len(actualVariables))
	for i, variable := range actualVariables {
		localVariable := make(map[string]interface{})
		if v, ok := variable.GetTypeOk(); ok {
			localVariable["type"] = *v
		}
		if v, ok := variable.GetNameOk(); ok {
			localVariable["name"] = *v
		}
		if v, ok := variable.GetExampleOk(); ok {
			localVariable["example"] = *v
		}
		if v, ok := variable.GetIdOk(); ok {
			localVariable["id"] = *v
		}
		if v, ok := variable.GetPatternOk(); ok {
			localVariable["pattern"] = *v
		}
		localBrowserVariables[i] = localVariable
	}

	if err := d.Set("browser_variable", localBrowserVariables); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("device_ids", syntheticsTest.GetOptions().DeviceIds); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("locations", syntheticsTest.Locations); err != nil {
		return diag.FromErr(err)
	}

	actualOptions := syntheticsTest.GetOptions()
	localOptionsList := make(map[string]interface{})
	if actualOptions.HasFollowRedirects() {
		localOptionsList["follow_redirects"] = actualOptions.GetFollowRedirects()
	}
	if actualOptions.HasMinFailureDuration() {
		localOptionsList["min_failure_duration"] = actualOptions.GetMinFailureDuration()
	}
	if actualOptions.HasMinLocationFailed() {
		localOptionsList["min_location_failed"] = actualOptions.GetMinLocationFailed()
	}
	if actualOptions.HasTickEvery() {
		localOptionsList["tick_every"] = actualOptions.GetTickEvery()
	}
	if actualOptions.HasAcceptSelfSigned() {
		localOptionsList["accept_self_signed"] = actualOptions.GetAcceptSelfSigned()
	}
	if actualOptions.HasAllowInsecure() {
		localOptionsList["allow_insecure"] = actualOptions.GetAllowInsecure()
	}
	if actualOptions.HasRetry() {
		retry := actualOptions.GetRetry()
		optionsListRetry := make(map[string]interface{})
		optionsListRetry["count"] = retry.GetCount()

		if interval, ok := retry.GetIntervalOk(); ok {
			optionsListRetry["interval"] = interval
		}

		localOptionsList["retry"] = []map[string]interface{}{optionsListRetry}
	}
	if actualOptions.HasMonitorOptions() {
		actualMonitorOptions := actualOptions.GetMonitorOptions()
		renotifyInterval := actualMonitorOptions.GetRenotifyInterval()

		optionsListMonitorOptions := make(map[string]int64)
		optionsListMonitorOptions["renotify_interval"] = renotifyInterval
		localOptionsList["monitor_options"] = []map[string]int64{optionsListMonitorOptions}
	}
	if actualOptions.HasNoScreenshot() {
		localOptionsList["no_screenshot"] = actualOptions.GetNoScreenshot()
	}
	if actualOptions.HasMonitorName() {
		localOptionsList["monitor_name"] = actualOptions.GetMonitorName()
	}
	if actualOptions.HasMonitorPriority() {
		localOptionsList["monitor_priority"] = actualOptions.GetMonitorPriority()
	}
	if actualOptions.HasRestrictedRoles() {
		localOptionsList["restricted_roles"] = actualOptions.GetRestrictedRoles()
	}
	if actualOptions.HasCi() {
		actualCi := actualOptions.GetCi()
		ciOptions := make(map[string]interface{})
		ciOptions["execution_rule"] = actualCi.GetExecutionRule()

		localOptionsList["ci"] = []map[string]interface{}{ciOptions}
	}

	if rumSettings, ok := actualOptions.GetRumSettingsOk(); ok {
		localRumSettings := make(map[string]interface{})
		localRumSettings["is_enabled"] = rumSettings.GetIsEnabled()

		if rumSettings.HasApplicationId() {
			localRumSettings["application_id"] = rumSettings.GetApplicationId()
		}

		if rumSettings.HasClientTokenId() {
			localRumSettings["client_token_id"] = rumSettings.GetClientTokenId()
		}

		localOptionsList["rum_settings"] = []map[string]interface{}{localRumSettings}
	}

	localOptionsLists := make([]map[string]interface{}, 1)
	localOptionsLists[0] = localOptionsList
	if err := d.Set("options_list", localOptionsLists); err != nil {
		return diag.FromErr(err)
	}

	steps := syntheticsTest.GetSteps()
	var localSteps []map[string]interface{}

	for stepIndex, step := range steps {
		localStep := make(map[string]interface{})
		localStep["name"] = step.GetName()
		localStep["type"] = string(step.GetType())
		localStep["timeout"] = step.GetTimeout()

		if allowFailure, ok := step.GetAllowFailureOk(); ok {
			localStep["allow_failure"] = allowFailure
		}

		if isCritical, ok := step.GetIsCriticalOk(); ok {
			localStep["is_critical"] = isCritical
		}

		localParams := make(map[string]interface{})
		params := step.GetParams()
		paramsMap := params.(map[string]interface{})

		for key, value := range paramsMap {
			localParams[convertStepParamsKey(key)] = convertStepParamsValueForState(convertStepParamsKey(key), value)
		}

		if elementParams, ok := localParams["element"]; ok {
			var stepElement interface{}
			utils.GetMetadataFromJSON([]byte(elementParams.(string)), &stepElement)

			if elementUserLocator, ok := stepElement.(map[string]interface{})["userLocator"]; ok {
				userLocator := elementUserLocator.(map[string]interface{})
				values := userLocator["values"]
				value := values.([]interface{})[0]

				localElementUserLocator := map[string]interface{}{
					"fail_test_on_cannot_locate": userLocator["failTestOnCannotLocate"],
					"value": []map[string]interface{}{
						value.(map[string]interface{}),
					},
				}

				localParams["element_user_locator"] = []map[string]interface{}{localElementUserLocator}
			}
		}

		localStep["params"] = []interface{}{localParams}

		if forceElementUpdate, ok := d.GetOk(fmt.Sprintf("browser_step.%d.force_element_update", stepIndex)); ok {
			localStep["force_element_update"] = forceElementUpdate
		}

		localSteps = append(localSteps, localStep)
	}

	if err := d.Set("browser_step", localSteps); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("name", syntheticsTest.GetName()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("message", syntheticsTest.GetMessage()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("status", syntheticsTest.GetStatus()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("tags", syntheticsTest.Tags); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("monitor_id", syntheticsTest.MonitorId); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func updateSyntheticsAPITestLocalState(d *schema.ResourceData, syntheticsTest *datadogV1.SyntheticsAPITest) diag.Diagnostics {
	if err := d.Set("type", syntheticsTest.GetType()); err != nil {
		return diag.FromErr(err)
	}
	if syntheticsTest.HasSubtype() {
		if err := d.Set("subtype", syntheticsTest.GetSubtype()); err != nil {
			return diag.FromErr(err)
		}
	}

	config := syntheticsTest.GetConfig()
	actualRequest := config.GetRequest()
	localRequest := buildLocalRequest(actualRequest)

	if syntheticsTest.GetSubtype() != "multi" {
		if err := d.Set("request_definition", []map[string]interface{}{localRequest}); err != nil {
			return diag.FromErr(err)
		}
	}
	if err := d.Set("request_headers", actualRequest.Headers); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("request_query", actualRequest.GetQuery()); err != nil {
		return diag.FromErr(err)
	}

	if basicAuth, ok := actualRequest.GetBasicAuthOk(); ok {
		localAuth := buildLocalBasicAuth(basicAuth)

		if err := d.Set("request_basicauth", []map[string]string{localAuth}); err != nil {
			return diag.FromErr(err)
		}
	}

	if clientCertificate, ok := actualRequest.GetCertificateOk(); ok {
		localCertificate := make(map[string][]map[string]string)
		localCertificate["cert"] = make([]map[string]string, 1)
		localCertificate["cert"][0] = make(map[string]string)
		localCertificate["key"] = make([]map[string]string, 1)
		localCertificate["key"][0] = make(map[string]string)

		cert := clientCertificate.GetCert()
		localCertificate["cert"][0]["filename"] = cert.GetFilename()

		key := clientCertificate.GetKey()
		localCertificate["key"][0]["filename"] = key.GetFilename()

		// the content of client certificate is write-only so it will not be returned by the API.
		// To avoid useless diff but also prevent storing the value in clear in the state
		// we store a hash of the value.
		if configCertificateContent, ok := d.GetOk("request_client_certificate.0.cert.0.content"); ok {
			localCertificate["cert"][0]["content"] = getCertificateStateValue(configCertificateContent.(string))
		}
		if configKeyContent, ok := d.GetOk("request_client_certificate.0.key.0.content"); ok {
			localCertificate["key"][0]["content"] = getCertificateStateValue(configKeyContent.(string))
		}

		if err := d.Set("request_client_certificate", []map[string][]map[string]string{localCertificate}); err != nil {
			return diag.FromErr(err)
		}
	}

	if proxy, ok := actualRequest.GetProxyOk(); ok {
		localProxy := make(map[string]interface{})
		localProxy["url"] = proxy.GetUrl()
		localProxy["headers"] = proxy.GetHeaders()

		d.Set("request_proxy", []map[string]interface{}{localProxy})
	}

	actualAssertions := config.GetAssertions()
	localAssertions, err := buildLocalAssertions(actualAssertions)

	if err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("assertion", localAssertions); err != nil {
		return diag.FromErr(err)
	}

	configVariables := config.GetConfigVariables()
	localConfigVariables := make([]map[string]interface{}, len(configVariables))
	for i, configVariable := range configVariables {
		localVariable := make(map[string]interface{})
		if v, ok := configVariable.GetTypeOk(); ok {
			localVariable["type"] = *v
		}
		if v, ok := configVariable.GetNameOk(); ok {
			localVariable["name"] = *v
		}

		if configVariable.GetType() != "global" {
			if v, ok := configVariable.GetExampleOk(); ok {
				localVariable["example"] = *v
			}
			if v, ok := configVariable.GetPatternOk(); ok {
				localVariable["pattern"] = *v
			}
		}
		if v, ok := configVariable.GetIdOk(); ok {
			localVariable["id"] = *v
		}
		localConfigVariables[i] = localVariable
	}

	if err := d.Set("config_variable", localConfigVariables); err != nil {
		return diag.FromErr(err)
	}

	if steps, ok := config.GetStepsOk(); ok {
		localSteps := make([]interface{}, len(*steps))

		for i, step := range *steps {
			localStep := make(map[string]interface{})
			localStep["name"] = step.GetName()
			localStep["subtype"] = step.GetSubtype()

			localAssertions, err := buildLocalAssertions(step.GetAssertions())
			if err != nil {
				return diag.FromErr(err)
			}
			localStep["assertion"] = localAssertions
			localStep["extracted_value"] = buildLocalExtractedValues(step.GetExtractedValues())

			stepRequest := step.GetRequest()
			localRequest := buildLocalRequest(stepRequest)
			localRequest["allow_insecure"] = stepRequest.GetAllowInsecure()
			localRequest["follow_redirects"] = stepRequest.GetFollowRedirects()
			localStep["request_definition"] = []map[string]interface{}{localRequest}
			localStep["request_headers"] = stepRequest.GetHeaders()
			localStep["request_query"] = stepRequest.GetQuery()

			if basicAuth, ok := stepRequest.GetBasicAuthOk(); ok {
				localAuth := buildLocalBasicAuth(basicAuth)
				localStep["request_basicauth"] = []map[string]string{localAuth}
			}

			if clientCertificate, ok := stepRequest.GetCertificateOk(); ok {
				localCertificate := make(map[string][]map[string]string)
				localCertificate["cert"] = make([]map[string]string, 1)
				localCertificate["cert"][0] = make(map[string]string)
				localCertificate["key"] = make([]map[string]string, 1)
				localCertificate["key"][0] = make(map[string]string)

				cert := clientCertificate.GetCert()
				localCertificate["cert"][0]["filename"] = cert.GetFilename()

				key := clientCertificate.GetKey()
				localCertificate["key"][0]["filename"] = key.GetFilename()

				certContentKey := fmt.Sprintf("api_step.%d.request_client_certificate.0.cert.0.content", i)
				keyContentKey := fmt.Sprintf("api_step.%d.request_client_certificate.0.key.0.content", i)

				// the content of client certificate is write-only so it will not be returned by the API.
				// To avoid useless diff but also prevent storing the value in clear in the state
				// we store a hash of the value.
				if configCertificateContent, ok := d.GetOk(certContentKey); ok {
					localCertificate["cert"][0]["content"] = getCertificateStateValue(configCertificateContent.(string))
				}
				if configKeyContent, ok := d.GetOk(keyContentKey); ok {
					localCertificate["key"][0]["content"] = getCertificateStateValue(configKeyContent.(string))
				}

				localStep["request_client_certificate"] = []map[string][]map[string]string{localCertificate}
			}

			if proxy, ok := stepRequest.GetProxyOk(); ok {
				localProxy := make(map[string]interface{})
				localProxy["url"] = proxy.GetUrl()
				localProxy["headers"] = proxy.GetHeaders()

				localStep["request_proxy"] = []map[string]interface{}{localProxy}
			}

			localStep["allow_failure"] = step.GetAllowFailure()
			localStep["is_critical"] = step.GetIsCritical()

			if retry, ok := step.GetRetryOk(); ok {
				localRetry := make(map[string]interface{})
				if count, ok := retry.GetCountOk(); ok {
					localRetry["count"] = *count
				}
				if interval, ok := retry.GetIntervalOk(); ok {
					localRetry["interval"] = *interval
				}
				localStep["retry"] = []map[string]interface{}{localRetry}
			}

			localSteps[i] = localStep
		}

		if err := d.Set("api_step", localSteps); err != nil {
			return diag.FromErr(err)
		}
	}

	if err := d.Set("device_ids", syntheticsTest.GetOptions().DeviceIds); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("locations", syntheticsTest.Locations); err != nil {
		return diag.FromErr(err)
	}

	actualOptions := syntheticsTest.GetOptions()
	localOptionsList := make(map[string]interface{})
	if actualOptions.HasFollowRedirects() {
		localOptionsList["follow_redirects"] = actualOptions.GetFollowRedirects()
	}
	if actualOptions.HasMinFailureDuration() {
		localOptionsList["min_failure_duration"] = actualOptions.GetMinFailureDuration()
	}
	if actualOptions.HasMinLocationFailed() {
		localOptionsList["min_location_failed"] = actualOptions.GetMinLocationFailed()
	}
	if actualOptions.HasTickEvery() {
		localOptionsList["tick_every"] = actualOptions.GetTickEvery()
	}
	if actualOptions.HasAcceptSelfSigned() {
		localOptionsList["accept_self_signed"] = actualOptions.GetAcceptSelfSigned()
	}
	if actualOptions.HasCheckCertificateRevocation() {
		localOptionsList["check_certificate_revocation"] = actualOptions.GetCheckCertificateRevocation()
	}
	if actualOptions.HasAllowInsecure() {
		localOptionsList["allow_insecure"] = actualOptions.GetAllowInsecure()
	}
	if actualOptions.HasRetry() {
		retry := actualOptions.GetRetry()
		optionsListRetry := make(map[string]interface{})
		optionsListRetry["count"] = retry.GetCount()

		if interval, ok := retry.GetIntervalOk(); ok {
			optionsListRetry["interval"] = interval
		}

		localOptionsList["retry"] = []map[string]interface{}{optionsListRetry}
	}
	if actualOptions.HasMonitorOptions() {
		actualMonitorOptions := actualOptions.GetMonitorOptions()
		renotifyInterval := actualMonitorOptions.GetRenotifyInterval()

		optionsListMonitorOptions := make(map[string]int64)
		optionsListMonitorOptions["renotify_interval"] = renotifyInterval
		localOptionsList["monitor_options"] = []map[string]int64{optionsListMonitorOptions}
	}
	if actualOptions.HasMonitorName() {
		localOptionsList["monitor_name"] = actualOptions.GetMonitorName()
	}
	if actualOptions.HasMonitorPriority() {
		localOptionsList["monitor_priority"] = actualOptions.GetMonitorPriority()
	}
	if actualOptions.HasRestrictedRoles() {
		localOptionsList["restricted_roles"] = actualOptions.GetRestrictedRoles()
	}
	if actualOptions.HasCi() {
		actualCi := actualOptions.GetCi()
		ciOptions := make(map[string]interface{})
		ciOptions["execution_rule"] = actualCi.GetExecutionRule()

		localOptionsList["ci"] = []map[string]interface{}{ciOptions}
	}

	localOptionsLists := make([]map[string]interface{}, 1)
	localOptionsLists[0] = localOptionsList
	if err := d.Set("options_list", localOptionsLists); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("name", syntheticsTest.GetName()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("message", syntheticsTest.GetMessage()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("status", syntheticsTest.GetStatus()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("tags", syntheticsTest.Tags); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("monitor_id", syntheticsTest.MonitorId); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func convertToString(i interface{}) string {
	switch v := i.(type) {
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		return v
	case datadogV1.HTTPMethod:
		return string(v)
	default:
		// TODO: manage target for JSON body assertions
		valStrr, err := json.Marshal(v)
		if err == nil {
			return string(valStrr)
		}
		return ""
	}
}

func setFloatTargetValue(subTarget *datadogV1.SyntheticsAssertionJSONPathTargetTarget, value string) {
	if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
		subTarget.SetTargetValue(floatValue)
	}
}

func validateSyntheticsAssertionOperator(val interface{}, key string) (warns []string, errs []error) {
	_, err := datadogV1.NewSyntheticsAssertionOperatorFromValue(val.(string))
	if err != nil {
		_, err2 := datadogV1.NewSyntheticsAssertionJSONPathOperatorFromValue(val.(string))
		if err2 != nil {
			errs = append(errs, err, err2)
		}
	}
	return
}

func isCertHash(content string) bool {
	// a sha256 hash consists of 64 hexadecimal characters
	isHash, _ := regexp.MatchString("^[A-Fa-f0-9]{64}$", content)

	return isHash
}

// get the sha256 of a client certificate content
// in some case where Terraform compares the state value
// we already get the hashed value so we don't need to
// hash it again
func getCertificateStateValue(content string) string {
	if isHash := isCertHash(content); isHash {
		return content
	}

	return utils.ConvertToSha256(content)
}

func getParamsKeysForStepType(stepType datadogV1.SyntheticsStepType) []string {
	switch stepType {
	case datadogV1.SYNTHETICSSTEPTYPE_ASSERT_CURRENT_URL:
		return []string{"check", "value"}

	case datadogV1.SYNTHETICSSTEPTYPE_ASSERT_ELEMENT_ATTRIBUTE:
		return []string{"attribute", "check", "element", "value"}

	case datadogV1.SYNTHETICSSTEPTYPE_ASSERT_ELEMENT_CONTENT:
		return []string{"check", "element", "value"}

	case datadogV1.SYNTHETICSSTEPTYPE_ASSERT_ELEMENT_PRESENT:
		return []string{"element"}

	case datadogV1.SYNTHETICSSTEPTYPE_ASSERT_EMAIL:
		return []string{"email"}

	case datadogV1.SYNTHETICSSTEPTYPE_ASSERT_FILE_DOWNLOAD:
		return []string{"file"}

	case datadogV1.SYNTHETICSSTEPTYPE_ASSERT_FROM_JAVASCRIPT:
		return []string{"code", "element"}

	case datadogV1.SYNTHETICSSTEPTYPE_ASSERT_PAGE_CONTAINS:
		return []string{"value"}

	case datadogV1.SYNTHETICSSTEPTYPE_ASSERT_PAGE_LACKS:
		return []string{"value"}

	case datadogV1.SYNTHETICSSTEPTYPE_CLICK:
		return []string{"click_type", "element"}

	case datadogV1.SYNTHETICSSTEPTYPE_EXTRACT_FROM_JAVASCRIPT:
		return []string{"code", "element", "variable"}

	case datadogV1.SYNTHETICSSTEPTYPE_EXTRACT_VARIABLE:
		return []string{"element", "variable"}

	case datadogV1.SYNTHETICSSTEPTYPE_GO_TO_EMAIL_LINK:
		return []string{"value"}

	case datadogV1.SYNTHETICSSTEPTYPE_GO_TO_URL:
		return []string{"value"}

	case datadogV1.SYNTHETICSSTEPTYPE_HOVER:
		return []string{"element"}

	case datadogV1.SYNTHETICSSTEPTYPE_PLAY_SUB_TEST:
		return []string{"playing_tab_id", "subtest_public_id"}

	case datadogV1.SYNTHETICSSTEPTYPE_PRESS_KEY:
		return []string{"modifiers", "value"}

	case datadogV1.SYNTHETICSSTEPTYPE_REFRESH:
		return []string{}

	case datadogV1.SYNTHETICSSTEPTYPE_RUN_API_TEST:
		return []string{"request"}

	case datadogV1.SYNTHETICSSTEPTYPE_SCROLL:
		return []string{"element", "x", "y"}

	case datadogV1.SYNTHETICSSTEPTYPE_SELECT_OPTION:
		return []string{"element", "value"}

	case datadogV1.SYNTHETICSSTEPTYPE_TYPE_TEXT:
		return []string{"delay", "element", "value"}

	case datadogV1.SYNTHETICSSTEPTYPE_UPLOAD_FILES:
		return []string{"element", "files", "with_click"}

	case datadogV1.SYNTHETICSSTEPTYPE_WAIT:
		return []string{"value"}
	}

	return []string{}
}

func convertStepParamsValueForConfig(stepType datadogV1.SyntheticsStepType, key string, value interface{}) interface{} {
	switch key {
	case "element", "email", "file", "files", "request":
		var result interface{}
		if err := utils.GetMetadataFromJSON([]byte(value.(string)), &result); err != nil {
			log.Printf("[ERROR] Error converting step param %s: %v", key, err)
		}
		return result

	case "playing_tab_id":
		result, _ := strconv.Atoi(value.(string))
		return result

	case "value":
		if stepType == datadogV1.SYNTHETICSSTEPTYPE_WAIT {
			result, _ := strconv.Atoi(value.(string))
			return result
		}

		return value

	case "variable":
		return value.([]interface{})[0]
	}

	return value
}

func convertStepParamsValueForState(key string, value interface{}) interface{} {
	switch key {
	case "element", "email", "file", "files", "request":
		result, _ := json.Marshal(value)
		return string(result)

	case "playing_tab_id", "value":
		return convertToString(value)

	case "variable":
		return []interface{}{value}
	}

	return value
}

func convertStepParamsKey(key string) string {
	switch key {
	case "click_type":
		return "clickType"

	case "clickType":
		return "click_type"

	case "playing_tab_id":
		return "playingTabId"

	case "playingTabId":
		return "playing_tab_id"

	case "subtest_public_id":
		return "subtestPublicId"

	case "subtestPublicId":
		return "subtest_public_id"

	case "with_click":
		return "withClick"

	case "withClick":
		return "with_click"
	}

	return key
}
