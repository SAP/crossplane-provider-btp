import * as xp from './crossplane-helpers.js';
import { Trend } from 'k6/metrics';

const xpTimeToReady = new Trend('xp_time_to_ready', true);
const xpTimeToDelete = new Trend('xp_time_to_delete', true);

export const options = { iterations: 1, vus: 1 };

const READY_TIMEOUT = Number(__ENV.XP_DIADROMOS_BTP_READY_TIMEOUT || '1800');
const DELETE_TIMEOUT = Number(__ENV.XP_DIADROMOS_BTP_DELETE_TIMEOUT || '1800');
const REGION = __ENV.XP_DIADROMOS_BTP_REGION || 'eu10';
const NAME_PREFIX = 'xp-btp-bench';
const SUBDOMAIN_PREFIX = 'xpbtpbench';

function safePart(value) {
  return value.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/^-+|-+$/g, '').slice(0, 20);
}

export default function () {
  const suffix = `${safePart(__ENV.GITHUB_RUN_ID || Date.now().toString(36))}-${safePart(__ENV.GITHUB_RUN_ATTEMPT || '1')}-${Date.now().toString(36)}`.slice(-35);
  const name = `${NAME_PREFIX}-${suffix}`.slice(0, 63).replace(/-+$/g, '');
  const subdomain = `${SUBDOMAIN_PREFIX}-${suffix}`.slice(0, 63).replace(/-+$/g, '');
  const apiVersion = 'account.btp.sap.crossplane.io/v1alpha1';
  const manifest = {
    apiVersion,
    kind: 'Subaccount',
    metadata: { namespace: 'default', name },
    spec: {
      providerConfigRef: { name: 'default' },
      forProvider: { displayName: name, region: REGION, subdomain },
    },
  };

  const client = xp.k8sClient();
  let created = false;
  let originalError;
  let deletionError;
  const createStarted = Date.now();
  try {
    xp.measureOperation('create', 'Subaccount', () => {
      client.create(manifest);
      created = true;
      xp.xpResourcesCreated.add(1);
      xp.waitForReady('Subaccount', name, { namespace: 'default', apiVersion, timeout: READY_TIMEOUT });
      xpTimeToReady.add(Date.now() - createStarted, { resource_kind: 'Subaccount' });
    });
  } catch (error) {
    originalError = error;
  }

  finally {
    if (created) {
      try {
        xp.deleteAndWait('Subaccount', name, {
          namespace: 'default', apiVersion, timeout: DELETE_TIMEOUT,
          trendMetric: xpTimeToDelete, trendTags: { resource_kind: 'Subaccount' },
        });
      } catch (error) {
        xp.xpResourcesFailed.add(1);
        deletionError = error;
      }
    }
  }
  if (deletionError) {
    const detail = originalError ? `; original create/readiness failure: ${originalError}` : '';
    throw new Error(`Subaccount deletion failed for owned resource ${name}: ${deletionError}${detail}`);
  }
  if (originalError) throw originalError;
}
