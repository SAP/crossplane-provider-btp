#!/usr/bin/env node
const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');

const source = fs.readFileSync(new URL('./k6/subaccount-create-delete.js', `file://${__filename}`), 'utf8')
  .replace("import * as xp from './crossplane-helpers.js';", '')
  .replace("import { Trend } from 'k6/metrics';", '')
  .replace('export const options', 'const options')
  .replace('export default function ()', 'function workload()');

function harness({ createFailure, readyFailure, deleteFailure } = {}) {
  const created = [];
  const deleted = [];
  const ready = [];
  const metrics = [];
  const client = {
    create(manifest) {
      if (createFailure === manifest.kind) throw new Error('simulated create failure');
      created.push(manifest);
    },
  };
  const xp = {
    k8sClient: () => client,
    measureOperation(_op, _kind, fn) { return fn(); },
    waitForReady(kind, name) {
      ready.push(kind);
      if (readyFailure === kind) throw new Error('simulated readiness failure');
    },
    deleteAndWait(kind, name) {
      deleted.push({ kind, name });
      if (deleteFailure === kind) throw new Error('simulated delete failure');
    },
    xpResourcesCreated: { add() {} },
    xpResourcesFailed: { add() {} },
  };
  class Trend { add(value, tags) { metrics.push({ value, tags }); } }
  const context = {
    xp, Trend, __ENV: {
      XP_DIADROMOS_BTP_RUN_ID: 'offline-test',
      XP_DIADROMOS_BTP_SUBACCOUNT_ADMIN: 'benchmark@example.invalid',
      GITHUB_RUN_ATTEMPT: '1',
    }, Date, console,
  };
  vm.runInNewContext(`${source}\nthis.runWorkload = workload;`, context);
  return { run: context.runWorkload, created, deleted, ready, metrics };
}

const kinds = ['Subaccount', 'Directory', 'Entitlement', 'DirectoryEntitlement', 'SubaccountApiCredential'];
const happy = harness();
happy.run();
assert.deepEqual(happy.created.map((x) => x.kind), kinds);
assert.deepEqual(happy.ready, kinds);
assert.deepEqual(happy.deleted.map((x) => x.kind), [...kinds].reverse());
assert.equal(new Set(happy.created.map((x) => x.metadata.name)).size, kinds.length);
assert.equal(happy.created[3].metadata.namespace, undefined, 'DirectoryEntitlement is cluster-scoped');
assert.equal(happy.created[4].spec.forProvider.readOnly, true);
assert.ok(happy.created[4].spec.writeConnectionSecretToRef.name.length <= 63);

const readyFail = harness({ readyFailure: 'Entitlement' });
assert.throws(() => readyFail.run(), /create\/readiness failed/);
assert.deepEqual(readyFail.deleted.map((x) => x.kind), ['Entitlement', 'Directory', 'Subaccount']);

const partialCreate = harness({ createFailure: 'Entitlement' });
assert.throws(() => partialCreate.run(), /create\/readiness failed/);
assert.deepEqual(partialCreate.deleted.map((x) => x.kind), ['Directory', 'Subaccount']);

const deleteFail = harness({ deleteFailure: 'DirectoryEntitlement' });
assert.throws(() => deleteFail.run(), /delete failed for owned DirectoryEntitlement/);
assert.deepEqual(deleteFail.deleted.map((x) => x.kind), [...kinds].reverse(), 'cleanup continues after a delete error');

console.log('Workload manifest and cleanup fixtures passed.');
