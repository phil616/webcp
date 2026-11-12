// SPDX-License-Identifier: GPL-3.0-only
// Adapted from 1Panel frontend/src/components/table/shared.ts, blob c12282f0c60c9154b6b36576280ededb5a4d5baa.
// Only operation rendering and tooltip helpers are retained; all permission hooks are removed.
import type { Component } from 'vue';
import type { ButtonProps, UseTooltipProps } from 'element-plus';
export interface FuTableOperationButton<Row = unknown> {
  key?: string | number;
  label?: string;
  command?: string;
  click?: (row: Row) => void;
  disabled?: boolean | ((row: Row) => boolean);
  show?: boolean | ((row: Row) => boolean);
  type?: ButtonProps['type'];
  icon?: Component;
  divided?: boolean;
  dot?: boolean | ((row: Row) => boolean);
}
export const TABLE_TOOLTIP_OPTIONS: Pick<UseTooltipProps, 'placement'> = { placement: 'bottom' };
export const resolveMaybeFn = <T, R>(value: R | ((row: T) => R), row: T): R =>
  typeof value === 'function' ? (value as (row: T) => R)(row) : value;
export const isOperationVisible = <T>(button: FuTableOperationButton<T>, row: T) =>
  resolveMaybeFn(button.show ?? true, row);
export const isOperationDisabled = <T>(button: FuTableOperationButton<T>, row: T) =>
  Boolean(resolveMaybeFn(button.disabled ?? false, row));
