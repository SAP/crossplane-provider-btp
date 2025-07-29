package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Subaccounts contains a array of value filled with subaccounts
type Subaccounts struct {
	Value []struct {
		Guid                          string           `json:"guid"`
		TechnicalName                 string           `json:"technicalName"`
		DisplayName                   string           `json:"displayName"`
		GlobalAccountGUID             string           `json:"globalAccountGUID"`
		ParentGUID                    string           `json:"parentGUID"`
		ParentType                    string           `json:"parentType"`
		Region                        string           `json:"region"`
		Subdomain                     string           `json:"subdomain"`
		BetaEnabled                   bool             `json:"betaEnabled"`
		UsedForProduction             string           `json:"usedForProduction"`
		Description                   string           `json:"description"`
		State                         string           `json:"state"`
		StateMessage                  string           `json:"stateMessage"`
		ContentAutomationState        interface{}      `json:"contentAutomationState"`
		ContentAutomationStateDetails interface{}      `json:"contentAutomationStateDetails"`
		CreatedDate                   int64            `json:"createdDate"`
		CreatedBy                     string           `json:"createdBy"`
		ModifiedDate                  int64            `json:"modifiedDate"`
		CustomProperties              CustomProperties `json:"customProperties,omitempty"`
		Labels                        Labels           `json:"labels,omitempty"`
	} `json:"value"`
}

// Labels contains the labels of a subaccount/directory
type Labels struct {
	SafeToDelete                          []string `json:"safe-to-delete,omitempty"`
	BuildId                               []string `json:"BUILD_ID,omitempty"`
	OrchestrateCloudSapSubaccountOperator []string `json:"orchestrate.cloud.sap/subaccount-operator,omitempty"`
}

// UaaAuth contains the data from the uaaauth response
type UaaAuth struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	Jti         string `json:"jti"`
}

// TechnicalUser contains the data of the technical user
type TechnicalUser struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// CisBinding contains the data from the cis binding of a subaccount
type CisBinding struct {
	Endpoints struct {
		AccountContextServiceUrl    string `json:"account_context_service_url"`
		AccountsServiceUrl          string `json:"accounts_service_url"`
		CloudAutomationUrl          string `json:"cloud_automation_url"`
		EntitlementsServiceUrl      string `json:"entitlements_service_url"`
		EventsServiceUrl            string `json:"events_service_url"`
		ExternalProviderRegistryUrl string `json:"external_provider_registry_url"`
		MetadataServiceUrl          string `json:"metadata_service_url"`
		OrderProcessingUrl          string `json:"order_processing_url"`
		ProvisioningServiceUrl      string `json:"provisioning_service_url"`
		SaasRegistryServiceUrl      string `json:"saas_registry_service_url"`
	} `json:"endpoints"`
	GrantType       string `json:"grant_type"`
	SapCloudService string `json:"sap.cloud.service"`
	Uaa             struct {
		Apiurl            string `json:"apiurl"`
		Clientid          string `json:"clientid"`
		Clientsecret      string `json:"clientsecret"`
		CredentialType    string `json:"credential-type"`
		Identityzone      string `json:"identityzone"`
		Identityzoneid    string `json:"identityzoneid"`
		Sburl             string `json:"sburl"`
		ServiceInstanceId string `json:"serviceInstanceId"`
		Subaccountid      string `json:"subaccountid"`
		Tenantid          string `json:"tenantid"`
		Tenantmode        string `json:"tenantmode"`
		Uaadomain         string `json:"uaadomain"`
		Url               string `json:"url"`
		Verificationkey   string `json:"verificationkey"`
		Xsappname         string `json:"xsappname"`
		Xsmasterappname   string `json:"xsmasterappname"`
		Zoneid            string `json:"zoneid"`
	} `json:"uaa"`
}

// CustomProperties contains the labels and AccountGUID
type CustomProperties []struct {
	AccountGUID string `json:"accountGUID"`
	Key         string `json:"key"`
	Value       string `json:"value"`
}

// DirectoryResponse contains the response message from getting the directories
type DirectoryResponse struct {
	Guid              string           `json:"guid"`
	ParentType        string           `json:"parentType"`
	GlobalAccountGUID string           `json:"globalAccountGUID"`
	DisplayName       string           `json:"displayName"`
	CreatedDate       int64            `json:"createdDate"`
	CreatedBy         string           `json:"createdBy"`
	ModifiedDate      int64            `json:"modifiedDate"`
	EntityState       string           `json:"entityState"`
	StateMessage      string           `json:"stateMessage"`
	DirectoryType     string           `json:"directoryType"`
	DirectoryFeatures []string         `json:"directoryFeatures"`
	CustomProperties  CustomProperties `json:"customProperties"`
	Labels            Labels           `json:"labels"`
	ContractStatus    string           `json:"contractStatus"`
	ConsumptionBased  bool             `json:"consumptionBased"`
	ParentGuid        string           `json:"parentGuid"`
	ParentGUID        string           `json:"parentGUID"`
}

// BtpSecuritySecret contains the respsonse if the security/api-credential btp call
type BtpSecuritySecret struct {
	Tenantmode        string `json:"tenantmode"`
	Subaccountid      string `json:"subaccountid"`
	CredentialType    string `json:"credential-type"`
	Clientid          string `json:"clientid"`
	Tokenurl          string `json:"tokenurl"`
	Xsappname         string `json:"xsappname"`
	Clientsecret      string `json:"clientsecret"`
	ServiceInstanceId string `json:"serviceInstanceId"`
	Url               string `json:"url"`
	Uaadomain         string `json:"uaadomain"`
	Apiurl            string `json:"apiurl"`
	Identityzone      string `json:"identityzone"`
	Identityzoneid    string `json:"identityzoneid"`
	Tenantid          string `json:"tenantid"`
	Name              string `json:"name"`
	Zoneid            string `json:"zoneid"`
	ReadOnly          bool   `json:"read-only"`
}

// TrustConfiguration contains a Globalaccount trust configurations (subset of relevant information)
type TrustConfiguration struct {
	OriginKey string `json:"originKey"`
	Name      string `json:"name"`
}

// Children contains an array of child directories from a global account.
type Children []struct {
	Guid              string           `json:"guid"`
	ParentGuid        string           `json:"parentGuid"`
	ParentGUID        string           `json:"parentGUID"`
	ParentType        string           `json:"parentType"`
	GlobalAccountGUID string           `json:"globalAccountGUID"`
	DisplayName       string           `json:"displayName"`
	CreatedDate       string           `json:"createdDate"`
	CreatedBy         string           `json:"createdBy"`
	ModifiedDate      string           `json:"modifiedDate"`
	Children          Children         `json:"children,omitempty"`
	EntityState       string           `json:"entityState"`
	StateMessage      string           `json:"stateMessage"`
	DirectoryType     string           `json:"directoryType"`
	DirectoryFeatures []string         `json:"directoryFeatures"`
	CustomProperties  CustomProperties `json:"customProperties"`
	Labels            Labels           `json:"labels"`
	ContractStatus    string           `json:"contractStatus"`
}

// GlobalaccountHiararchy contains the structure of the Globalaccount
type GlobalaccountHiararchy struct {
	CommercialModel  string   `json:"commercialModel"`
	ConsumptionBased bool     `json:"consumptionBased"`
	LicenseType      string   `json:"licenseType"`
	GeoAccess        string   `json:"geoAccess"`
	CostCenter       string   `json:"costCenter"`
	UseFor           string   `json:"useFor"`
	Origin           string   `json:"origin"`
	Guid             string   `json:"guid"`
	DisplayName      string   `json:"displayName"`
	Description      string   `json:"description"`
	CreatedDate      string   `json:"createdDate"`
	ModifiedDate     string   `json:"modifiedDate"`
	Children         Children `json:"children,omitempty"`
	EntityState      string   `json:"entityState"`
	StateMessage     string   `json:"stateMessage"`
	Subdomain        string   `json:"subdomain"`
	Subaccounts      []struct {
		Guid              string `json:"guid"`
		TechnicalName     string `json:"technicalName"`
		DisplayName       string `json:"displayName"`
		GlobalAccountGUID string `json:"globalAccountGUID"`
		ParentGUID        string `json:"parentGUID"`
		ParentType        string `json:"parentType"`
		Region            string `json:"region"`
		Subdomain         string `json:"subdomain"`
		BetaEnabled       bool   `json:"betaEnabled"`
		UsedForProduction string `json:"usedForProduction"`
		Description       string `json:"description"`
		State             string `json:"state"`
		StateMessage      string `json:"stateMessage"`
		CreatedDate       string `json:"createdDate"`
		CreatedBy         string `json:"createdBy"`
		ModifiedDate      string `json:"modifiedDate"`
	} `json:"subaccounts,omitempty"`
	ContractStatus string `json:"contractStatus"`
}

// GetUaaAuth uses the CIS binding to get a uaa token.
// It returns a struct with the token (UaaAuth)
func GetUaaAuth(cisBinding CisBinding) (*UaaAuth, error) {
	//configure parameters etc. for api call
	params := url.Values{}
	params.Add("grant_type", "client_credentials")
	auth := cisBinding.Uaa.Clientid + ":" + cisBinding.Uaa.Clientsecret
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	baseURL := cisBinding.Uaa.Url + "/oauth/token"

	req, err := http.NewRequest("GET", baseURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Add("Authorization", authHeader)

	//make api call to get uaa token
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	//parse response
	var uaaAuth UaaAuth
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&uaaAuth); err != nil {
			return nil, fmt.Errorf("error decoding JSON response: %w", err)
		}
	} else {
		return nil, fmt.Errorf("request failed with status code: %d\n", resp.StatusCode)
	}
	return &uaaAuth, nil
}

// GetUaaAuthForTrustConfiguration uses the BTP CLI to get a uaa token.
// It returns a struct with the token (UaaAuth)
func GetUaaAuthForTrustConfiguration(secret BtpSecuritySecret) (*UaaAuth, error) {

	//configure parameters etc. for api call
	params := url.Values{}
	params.Add("grant_type", "client_credentials")
	auth := secret.Clientid + ":" + secret.Clientsecret
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	baseURL := secret.Tokenurl + "/oauth/token"

	req, err := http.NewRequest("GET", baseURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Add("Authorization", authHeader)

	//make api call to get uaa token
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	//parse response
	var uaaAuth UaaAuth
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&uaaAuth); err != nil {
			return nil, fmt.Errorf("error decoding JSON response: %w", err)
		}
	} else {
		return nil, fmt.Errorf("request failed with status code: %d\n", resp.StatusCode)
	}
	return &uaaAuth, nil
}

// GetSubaccounts uses the uaa token and cis binding to get subaccounts of a globalaccount.
// Returns the Subaccounts struct
func GetSubaccounts(uaaAuth *UaaAuth, cisBinding CisBinding) (*Subaccounts, error) {
	//configure parameters etc. for api call
	baseURL := cisBinding.Endpoints.AccountsServiceUrl + "/accounts/v1/subaccounts"
	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+uaaAuth.AccessToken)
	req.Header.Add("Accept", "application/json")

	//make api call to get subaccounts
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	//parse response
	var subaccounts Subaccounts
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&subaccounts); err != nil {
			return nil, fmt.Errorf("error decoding JSON response: %w", err)
		}

	} else {
		return nil, fmt.Errorf("request failed with status code: %d\n", resp.StatusCode)
	}
	return &subaccounts, nil
}

// DeleteSubaccount uses the guid, cis binding and uaa token to delete a subaccount.
// Returns an error if it fails
func DeleteSubaccount(guid string, cisBinding CisBinding, uaaAuth *UaaAuth) error {
	fmt.Println("try to delete subaccount with guid: ", guid)
	//configure parameters etc. for api call
	baseURL := cisBinding.Endpoints.AccountsServiceUrl + "/accounts/v1/subaccounts/" + guid
	params := url.Values{}
	params.Add("forceDelete", "true")

	req, err := http.NewRequest("DELETE", baseURL+"?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+uaaAuth.AccessToken)
	req.Header.Add("Accept", "application/json")

	//make api call to delete an subaccount
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// CleanUpSubaccounts takes UaaAuth, CisBinding and Subaccounts to delete subaccounts.
func CleanUpSubaccounts(uaaAuth *UaaAuth, cisBinding CisBinding, subaccounts *Subaccounts) {
	//slice into single subaccounts
	for _, subaccount := range subaccounts.Value {
		buildId := os.Getenv("BUILD_ID")
		// check if the subaccounts is from the current build
		if len(subaccount.Labels.BuildId) > 0 && subaccount.Labels.BuildId[0] == buildId {
			//delete subaccount
			err := DeleteSubaccount(subaccount.Guid, cisBinding, uaaAuth)
			if err != nil {
				fmt.Printf("error deleting subaccount %s: %s", subaccount.DisplayName, err)
				return
			}
		}
	}
}

// btpLogin logs the in to the btp cli.
// returns error if it fails
func btpLogin(username string, password string, globalAccount string) error {
	// run command to login
	cmd := exec.Command("btp", "login", "--user", username, "--password", password, "--subdomain", globalAccount)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("login command failed: %s, output: %s", err, out.String())
	}
	return nil
}

// btpLogout logs out the user from the cli.
// returns error if it fails
func btpLogout() error {
	// run command to login
	cmd := exec.Command("btp", "logout")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("logout command failed: %s, output: %s", err, out.String())
	}
	return nil
}

// GetDirectoriesOfBuild uses an array of directory guids, CisBinding and UaaAuth to get directories of the current build.
// Returns array of DirectoryResponse and an error if it fails
func GetDirectoriesOfBuild(directoriesGuids []string, cisBinding CisBinding, uaaAuth UaaAuth) ([]DirectoryResponse, error) {
	var DirectoriesFromRun []DirectoryResponse
	// slice in to single directory's
	for _, directoryguid := range directoriesGuids {
		//configure parameters etc. for api call
		baseURL := cisBinding.Endpoints.AccountsServiceUrl + "/accounts/v1/directories/" + directoryguid
		req, err := http.NewRequest("GET", baseURL, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		req.Header.Add("Authorization", "Bearer "+uaaAuth.AccessToken)
		req.Header.Add("Accept", "application/json")

		//make api call to get data of the directory
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error making request: %w", err)
		}
		defer resp.Body.Close()

		//parse response
		var directory DirectoryResponse
		if resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&directory); err != nil {
				return nil, fmt.Errorf("error decoding JSON response: %w", err)
			}
		} else {
			return nil, fmt.Errorf("request failed with status code: %d\n", resp.StatusCode)
		}
		buildID := os.Getenv("BUILD_ID")
		// check if directory is from current build
		if len(directory.Labels.BuildId) > 0 && directory.Labels.BuildId[0] == buildID {
			DirectoriesFromRun = append(DirectoriesFromRun, directory)
		}
	}
	return DirectoriesFromRun, nil
}

// DeleteDirectories uses UaaAuth and CisBinding to delete the directories of current build.
// Returns error if it fails
func DeleteDirectories(uaaAuth *UaaAuth, cisBinding CisBinding) error {
	//get directories
	directoriesGuids, err := fetchAndPrintDirectoryGUIDs()
	if err != nil {
		return fmt.Errorf("error while fetching directories guids: %s\n", err)
	}

	//check if directories are from current run/build
	elementsToDelete, err := GetDirectoriesOfBuild(directoriesGuids, cisBinding, *uaaAuth)
	if err != nil {
		return err
	}

	//Delete directories from this Build
	for _, resource := range elementsToDelete {
		fmt.Printf("try to delete directory: %s\n", resource.DisplayName)
		//configure parameters etc. for api call
		baseURL := cisBinding.Endpoints.AccountsServiceUrl + "/accounts/v1/directories/" + resource.Guid
		params := url.Values{}
		params.Add("forceDelete", "true")

		req, err := http.NewRequest("DELETE", baseURL+"?"+params.Encode(), nil)
		if err != nil {
			return fmt.Errorf("error creating request: %w", err)
		}
		req.Header.Add("Authorization", "Bearer "+uaaAuth.AccessToken)
		req.Header.Add("Accept", "application/json")

		//make api call to delete directory
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error making request: %w", err)
		}
		defer resp.Body.Close()
		fmt.Printf("deleted directory: %s\n", resource.DisplayName)
	}
	return nil
}

// fetchAndPrintDirectoryGUIDs fetches all directories of the globalaccount.
// Returns array of strings that contains the guids of the directories or an error if it fails.
func fetchAndPrintDirectoryGUIDs() ([]string, error) {
	// Execute the BTP CLI command to get the account hierarchy
	cmd := exec.Command("btp", "--format", "json", "get", "accounts/global-account", "--show-hierarchy")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("command execution failed: %s, output: %s", err, out.String())
	}

	// parse the response
	var globalaccountHiararchy GlobalaccountHiararchy
	err = json.Unmarshal(out.Bytes(), &globalaccountHiararchy)
	if err != nil {
		return nil, err
	}
	//check if their are directories
	if globalaccountHiararchy.Children == nil {
		return nil, nil
	}

	// find directories and sub-directories
	directoriesGuids, err := findChildren(globalaccountHiararchy.Children)
	if err != nil {
		return nil, fmt.Errorf("error while finding children: %s\n", err)
	}
	return directoriesGuids, nil
}

// findChildren finds directories and sub-directories takes Children for that.
// Returns array of strings with the guid or an error if it fails.
func findChildren(children Children) ([]string, error) {
	var directoriesGuids []string
	buildID := os.Getenv("BUILD_ID")
	// slice childrens in single child
	for _, child := range children {
		//check if it's a folder/directory
		if child.DirectoryType == "FOLDER" {
			// check if it is from the current build
			for _, prop := range child.CustomProperties {
				if prop.Key == "BUILD_ID" && prop.Value == buildID {
					// add to array of guids
					directoriesGuids = append(directoriesGuids, child.Guid)
				}
			}
		}
		// check if their are sub children
		if child.Children != nil && len(child.Children) > 0 {
			// get guid of children
			childrenGUID, err := findChildren(child.Children)
			if err != nil {
				return nil, fmt.Errorf("error while finding children: %s\n", err)
			}
			for _, childGuid := range childrenGUID {
				directoriesGuids = append(directoriesGuids, childGuid)
			}
		}
	}
	return directoriesGuids, nil
}

// getTrustConfigurationForBuild uses btp cli to get the trust configuration of the globalaccount.
// Returns a filtered []TrustConfiguration for the buildID (naming pattern: <buildID><name>) or an error if it fails.
func getTrustConfigurationForBuild(buildID string) ([]TrustConfiguration, error) {

	// Execute the BTP CLI command to get the account hierarchy
	cmd := exec.Command("btp", "--format", "json", "list", "security/trust")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("command execution failed: %s, output: %s", err, out.String())
	}

	//parse response
	var trustConfigurationsList []TrustConfiguration
	if err := json.Unmarshal(out.Bytes(), &trustConfigurationsList); err != nil {
		return nil, fmt.Errorf("error decoding JSON response: %w", err)
	}

	// filter for trust configurations from current test build
	var trustConfigurationFromThisTestBuild []TrustConfiguration
	for _, tc := range trustConfigurationsList {
		if strings.HasPrefix(tc.Name, buildID) {
			trustConfigurationFromThisTestBuild = append(trustConfigurationFromThisTestBuild, tc)
		}
	}
	return trustConfigurationFromThisTestBuild, nil
}

// deleteTrustConfigurations uses an array of TrustConfigurationsList to delete the trust configurations.
// Returns an error if it fails.
func deleteTrustConfigurations(trustConfigurationsList []TrustConfiguration) error {
	// slice it in single trust configurations
	for _, trustConfiguration := range trustConfigurationsList {
		// delete trust configuration
		fmt.Println("try to delete: ", trustConfiguration.Name)
		cmd := exec.Command("btp", "delete", "security/trust", trustConfiguration.OriginKey, "--confirm")
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("failed deleting security/trust with name %s: command execution failed: %s, output: %s", trustConfiguration.Name, err, out.String())
		}
		fmt.Printf("deleted: %s\n", trustConfiguration.Name)
	}
	return nil
}

// checkIfAccountIsClean takes CisBinding and *UaaAuth to check if the globalaccount is cleaned.
// Returns true if its clean.
func checkIfAccountIsClean(cisBinding CisBinding, uaaAuth *UaaAuth) bool {
	directoriesGuids, err := fetchAndPrintDirectoryGUIDs()
	if err != nil {
		fmt.Printf("error while fetching directories guids: %s\n", err)
	}

	//check if directories are from current run/build
	elementsToDelete, err := GetDirectoriesOfBuild(directoriesGuids, cisBinding, *uaaAuth)
	if err != nil {
		fmt.Println("Error while get Directories of build")
	}
	if elementsToDelete != nil {
		// not all directories has been cleaned
		return false
	}

	subaccounts, err := GetSubaccounts(uaaAuth, cisBinding)
	if err != nil {
		fmt.Println("error while getting subaccounts: ", err)
		return false
	}
	for _, subaccount := range subaccounts.Value {
		buildId := os.Getenv("BUILD_ID")
		// check if the subaccounts is from the current build
		if len(subaccount.Labels.BuildId) > 0 && subaccount.Labels.BuildId[0] == buildId {
			return false
		}
	}
	return true
}

func cleanup() (errs error) {
	buildID := os.Getenv("BUILD_ID")

	fmt.Println("Get cis binding from env...")
	cisBindingEnv := os.Getenv("CIS_CENTRAL_BINDING")
	var cisBinding CisBinding
	if err := json.Unmarshal([]byte(cisBindingEnv), &cisBinding); err != nil {
		errs = errors.Join(errs, fmt.Errorf("error unmarshalling config JSON: %w", err))
		return errs
	}
	fmt.Println("Successfully got cis binding")

	//get uaa from envs
	fmt.Println("Get uaa auth from cis binding...")
	uaaAuth, err := GetUaaAuth(cisBinding)
	if err != nil {
		errs = errors.Join(errs, fmt.Errorf("error getting uaa auth: %w", err))
		return errs
	}
	fmt.Println("Successfully got uaa auth for cis binding")

	fmt.Println("Get btp technical user from env...")
	technicalUserEnv := os.Getenv("BTP_TECHNICAL_USER")
	var technicalUser TechnicalUser
	if err := json.Unmarshal([]byte(technicalUserEnv), &technicalUser); err != nil {
		errs = errors.Join(errs, fmt.Errorf("error unmarshalling config JSON: %w", err))
		return errs

	}
	fmt.Println("Successfully got BTP technical user")

	fmt.Println("Logging in to BTP CLI with technical user credentials...")
	err = btpLogin(technicalUser.Email, technicalUser.Password, cisBinding.Uaa.Identityzoneid)
	if err != nil {
		errs = errors.Join(errs, fmt.Errorf("error logging into BTP CLI: %w", err))
		return errs
	}
	defer func() {
		if tempErr := btpLogout(); tempErr != nil {
			errs = errors.Join(errs, fmt.Errorf("error logging out from BTP CLI: %w", tempErr))
		}
	}()
	fmt.Println("Successfully logged in to BTP CLI")

	fmt.Println("Get Trust Configuration for current build...")
	trustConfigurationsForCurrentBuild, err := getTrustConfigurationForBuild(buildID)
	if err != nil {
		errs = errors.Join(errs, fmt.Errorf("error getting trust Configurations: %w", err))
		return errs
	}
	fmt.Println("Successfully got Trust Configurations for current build")

	// delete trust confiurations for the current build
	fmt.Println("Deleting Trust Configurations for current build...")
	err = deleteTrustConfigurations(trustConfigurationsForCurrentBuild)
	if err != nil {
		errs = errors.Join(errs, fmt.Errorf("error while deleting trust configuration: %w", err))
		return errs
	}
	fmt.Println("Successfully deleted Trust Configurations for current build")

	// trying to delete subaccounts and directories of current build
	for i := 0; i < 5; i++ {
		fmt.Println("Trying to delete subaccounts and directories of current build... Attempt:", i+1)

		//delete directories for the current build
		err = DeleteDirectories(uaaAuth, cisBinding)
		if err != nil {
			fmt.Println("error while deleting Directory: ", err)
		}
		//delete subaccounts for the current build
		subaccounts, err := GetSubaccounts(uaaAuth, cisBinding)
		CleanUpSubaccounts(uaaAuth, cisBinding, subaccounts)
		if err != nil {
			fmt.Println("error while deleting directories: ", err)
		}
		//wait so child subaccounts or directories getting deleted
		time.Sleep(45 * time.Second)
		if checkIfAccountIsClean(cisBinding, uaaAuth) {
			fmt.Println("Globalaccount has been cleaned")
			return errs
		}
	}
	errs = errors.Join(errs, fmt.Errorf("globalaccount can not be cleaned in time"))
	return errs
}

func main() {
	if err := cleanup(); err != nil {
		fmt.Println("Cleanup failed:", err)
		os.Exit(1)
	} else {
		fmt.Println("Cleanup completed successfully")
		os.Exit(0)
	}
}
