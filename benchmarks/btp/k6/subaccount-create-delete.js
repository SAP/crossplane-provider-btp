import * as xp from './crossplane-helpers.js';
import { Trend } from 'k6/metrics';

const xpTimeToReady = new Trend('xp_time_to_ready', true);
const xpTimeToDelete = new Trend('xp_time_to_delete', true);
export const options = { iterations: 1, vus: 1 };

const READY_TIMEOUT = Number(__ENV.XP_DIADROMOS_BTP_READY_TIMEOUT || '600');
const DELETE_TIMEOUT = Number(__ENV.XP_DIADROMOS_BTP_DELETE_TIMEOUT || '600');
const REGION = __ENV.XP_DIADROMOS_BTP_REGION || 'eu10';
const SUBACCOUNT_ADMIN = __ENV.XP_DIADROMOS_BTP_SUBACCOUNT_ADMIN;
const RUN_ID = __ENV.XP_DIADROMOS_BTP_RUN_ID || __ENV.GITHUB_RUN_ID;
const NS = 'default';

function safePart(value) {
  return String(value).toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/^-+|-+$/g, '').slice(0, 16) || 'run';
}

function resource(kind, apiVersion, name, spec, namespaced = true) {
  return {
    apiVersion,
    kind,
    metadata: { ...(namespaced ? { namespace: NS } : {}), name },
    spec: {
      providerConfigRef: { name: 'default' },
      forProvider: spec,
      ...(kind === 'SubaccountApiCredential' ? {
        writeConnectionSecretToRef: { name: `${name.slice(0, 55)}-secret`, namespace: NS },
      } : {}),
    },
  };
}

export default function () {
  if (!SUBACCOUNT_ADMIN) throw new Error('XP_DIADROMOS_BTP_SUBACCOUNT_ADMIN is required');
  if (!RUN_ID) throw new Error('Set XP_DIADROMOS_BTP_RUN_ID (or GITHUB_RUN_ID) to identify owned resources');

  const suffix = `${safePart(RUN_ID)}-${safePart(__ENV.GITHUB_RUN_ATTEMPT || '1')}-${Date.now().toString(36)}`;
  const subName = `xp-btp-bench-${suffix}`.slice(0, 63).replace(/-+$/g, '');
  const dirName = `xp-btp-bench-dir-${suffix}`.slice(0, 63).replace(/-+$/g, '');
  const subdomain = `xpbtpbench-${suffix}`.slice(0, 63).replace(/-+$/g, '');
  const names = {
    Subaccount: subName,
    Directory: dirName,
    Entitlement: `xp-btp-bench-ent-${suffix}`.slice(0, 63).replace(/-+$/g, ''),
    DirectoryEntitlement: `xp-btp-bench-dirent-${suffix}`.slice(0, 63).replace(/-+$/g, ''),
    SubaccountApiCredential: `xp-btp-bench-api-${suffix}`.slice(0, 63).replace(/-+$/g, ''),
  };
  const accountAPI = 'account.btp.sap.crossplane.io/v1alpha1';
  const securityAPI = 'security.btp.sap.crossplane.io/v1alpha1';
  const manifests = [
    resource('Subaccount', accountAPI, names.Subaccount, {
      displayName: names.Subaccount, region: REGION, subdomain, subaccountAdmins: [SUBACCOUNT_ADMIN],
    }),
    resource('Directory', accountAPI, names.Directory, {
      description: `xp-diadromos benchmark ${suffix}`,
      directoryAdmins: [SUBACCOUNT_ADMIN],
      directoryFeatures: ['DEFAULT', 'ENTITLEMENTS'],
      displayName: names.Directory,
    }),
    resource('Entitlement', accountAPI, names.Entitlement, {
      serviceName: 'cis', servicePlanName: 'local', enable: true,
      subaccountRef: { name: names.Subaccount },
    }),
    resource('DirectoryEntitlement', accountAPI, names.DirectoryEntitlement, {
      directoryRef: { name: names.Directory }, serviceName: 'cis', planName: 'local',
    }, false),
    resource('SubaccountApiCredential', securityAPI, names.SubaccountApiCredential, {
      readOnly: true, subaccountRef: { name: names.Subaccount },
    }),
  ];

  const client = xp.k8sClient();
  const created = [];
  const errors = [];
  try {
    for (const manifest of manifests) {
      const kind = manifest.kind;
      const name = manifest.metadata.name;
      const started = Date.now();
      xp.measureOperation('create', kind, () => {
        client.create(manifest);
        // Track immediately after API create, before readiness can time out.
        created.push(manifest);
        xp.xpResourcesCreated.add(1);
        xp.waitForReady(kind, name, {
          namespace: manifest.metadata.namespace || NS,
          apiVersion: manifest.apiVersion,
          timeout: READY_TIMEOUT,
          requireReady: true,
        });
      });
      xpTimeToReady.add(Date.now() - started, { resource_kind: kind });
    }
  } catch (error) {
    errors.push(`create/readiness failed (${error?.name || 'error'})`);
  } finally {
    for (const manifest of created.reverse()) {
      const kind = manifest.kind;
      const name = manifest.metadata.name;
      try {
        xp.deleteAndWait(kind, name, {
          namespace: manifest.metadata.namespace || NS,
          apiVersion: manifest.apiVersion,
          timeout: DELETE_TIMEOUT,
          trendMetric: xpTimeToDelete,
          trendTags: { resource_kind: kind },
        });
      } catch (error) {
        xp.xpResourcesFailed.add(1);
        errors.push(`delete failed for owned ${kind}/${name} (${error?.name || 'error'})`);
      }
    }
  }
  if (errors.length) throw new Error(errors.join('; '));
}
