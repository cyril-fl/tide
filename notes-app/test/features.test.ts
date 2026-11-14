import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFileSync } from 'node:fs';
import { features, listedFeatures } from '../app/utils/features.ts';

test('each feature of the comparison has its label in French and English', () => {
  for (const locale of ['fr', 'en']) {
    const labels = JSON.parse(readFileSync(new URL(`../i18n/locales/${locale}.json`, import.meta.url), 'utf8'));
    for (const { group, features: listed } of listedFeatures(features)) {
      assert.ok(labels.settings.featureGroups[group], `${locale}: ${group}`);
      for (const feature of listed) assert.ok(labels.features[feature], `${locale}: ${feature}`);
    }
  }
});
