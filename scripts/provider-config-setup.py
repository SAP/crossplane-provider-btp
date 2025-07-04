import subprocess
import json
import time
import argparse

def run_command(command, parse_json=False, print_output=True):
    """Run a shell command, print the output, and optionally parse JSON."""
    try:
        result = subprocess.run(command, shell=True, check=True, stdout=subprocess.PIPE)
        output = result.stdout.decode('utf-8')

        if print_output:
            print(f"Command: {command}\nOutput:\n{output}\n")

        return json.loads(output) if parse_json else output
    except subprocess.CalledProcessError as e:
        print(f"Command failed with error: {e}")
        print(f"Command: {command}")
        print(f"Output: {e.output.decode('utf-8') if e.output else 'No output'}")
        raise

def get_btpenv_url(env):
    if env == 'live':
        btpCliUrl = 'https://cli.btp.cloud.sap'
    elif env == 'canary':
        btpCliUrl = 'https://canary.cli.btp.int.sap'
    else:
        raise ValueError(f"Unknown environment: {env}. Supported value for btpEnvUrl are live or canary")
    return btpCliUrl

def main(args):
    # Set environment variables from arguments
    btpEnvUrl        = get_btpenv_url(args.btpEnvName)
    userName         = args.userName
    password         = args.password
    subDomain        = args.subDomain
    subDomainAlias   = args.subDomainAlias
    region           = args.region
    
    run_command(f"btp login --url {btpEnvUrl} --user {userName} --password {password} --subdomain {subDomain}")

    run_command(f"btp --format json create accounts/subaccount --display-name {subDomainAlias}-cloud-mgmt --region {region} --subdomain {subDomainAlias}-cloud-mgmt --used-for-production false")
    time.sleep(30)

    subAccounts = run_command(f"btp --format json list accounts/subaccount --global-account {subDomain}", parse_json=True, print_output=False)

    technical_name = next((sa["technicalName"] for sa in subAccounts.get("value", []) if sa.get("displayName") == subDomainAlias + '-cloud-mgmt'), None)

    if technical_name:
        print(f"The technical Name for the subaccount '{subDomainAlias}-cloud-mgmt' is: {technical_name}")
    else:
        raise ValueError(f"No subaccount found with the display name '{subDomainAlias}-cloud-mgmt'.")

    commands = [
        f"btp assign accounts/entitlement --for-service cis --plan central --enable true --to-subaccount {technical_name}",
        f"""btp create services/instance --offering-name cis --service {subDomainAlias} --plan-name central --parameters '{{"grantType": "clientCredentials"}}' --subaccount {technical_name}""",
        f"btp create services/binding --name {subDomainAlias}-binding --instance-name {subDomainAlias} --subaccount {technical_name}"
    ]

    for command in commands:
        run_command(command)
        time.sleep(10)

    credentials_json = run_command(f"btp --format json get services/binding --name {subDomainAlias}-binding --subaccount {technical_name}", parse_json=True, print_output=False)
    
    print("Below json output should be uploaded to DwC vault namespace designated to the product")
    print(credentials_json["credentials"])

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Manage BTP subAccounts and services.")
    parser.add_argument("--btpEnvName", required=True, help="BTP environment canary or live")
    parser.add_argument("--userName", required=True, help="Technical Username for BTP login")
    parser.add_argument("--password", required=True, help="Technical Password for BTP login")
    parser.add_argument("--subDomain", required=True, help="Subdomain for the global account")
    parser.add_argument("--subDomainAlias", required=True, help="User friendly subdomain for the global account")
    parser.add_argument("--region", required=True, help="Region for the subaccount")

    args = parser.parse_args()
    main(args)
