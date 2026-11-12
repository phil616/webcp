<!-- SPDX-License-Identifier: GPL-3.0-only
Adapted from 1Panel frontend/src/components/codemirror-pro/index.vue, blob 35e7d9f01013fd8ebd82af5873d866c0b99bbc95.
Keep CodeMirror 6, oneDark, update listener and model contract. Add HTML/Lua and accessible labels;
use typed lifecycle/reconfiguration to avoid double initialization, leaked editors and lost focus.
-->
<script setup lang="ts">
import { ref, shallowRef, onMounted, onBeforeUnmount, watch } from 'vue';
import { basicSetup, EditorView } from 'codemirror';
import { EditorState, Compartment, type Extension } from '@codemirror/state';
import { oneDark } from '@codemirror/theme-one-dark';
import { StreamLanguage } from '@codemirror/language';
import { html } from '@codemirror/lang-html';
import { json } from '@codemirror/lang-json';
import { lua } from '@codemirror/legacy-modes/mode/lua';
import { nginx } from '@codemirror/legacy-modes/mode/nginx';
import { javascript } from '@codemirror/legacy-modes/mode/javascript';
import { css } from '@codemirror/legacy-modes/mode/css';
import { keymap, placeholder as editorPlaceholder } from '@codemirror/view';
import { defaultKeymap, indentWithTab } from '@codemirror/commands';
const props = withDefaults(
  defineProps<{
    modelValue: string;
    mode?: string;
    readonly?: boolean;
    disabled?: boolean;
    height?: number;
    minHeight?: number;
    label?: string;
    placeholder?: string;
    lineWrapping?: boolean;
  }>(),
  {
    mode: 'text',
    readonly: false,
    disabled: false,
    height: 400,
    minHeight: 200,
    label: '代码编辑器',
    placeholder: '',
    lineWrapping: false,
  },
);
const emit = defineEmits<{ 'update:modelValue': [value: string] }>();
const editorRef = ref<HTMLElement>();
const editorView = shallowRef<EditorView>();
const language = new Compartment();
const editable = new Compartment();
const readOnly = new Compartment();
let syncing = false;
function languageExtension(): Extension {
  switch (props.mode) {
    case 'html':
      return html();
    case 'lua':
      return StreamLanguage.define(lua);
    case 'nginx':
      return StreamLanguage.define(nginx);
    case 'json':
      return json();
    case 'javascript':
      return StreamLanguage.define(javascript);
    case 'css':
      return StreamLanguage.define(css);
    default:
      return [];
  }
}
onMounted(() => {
  if (!editorRef.value) return;
  editorView.value = new EditorView({
    parent: editorRef.value,
    state: EditorState.create({
      doc: props.modelValue,
      extensions: [
        basicSetup,
        oneDark,
        language.of(languageExtension()),
        editable.of(EditorView.editable.of(!props.disabled)),
        readOnly.of(EditorState.readOnly.of(props.readonly)),
        keymap.of([...defaultKeymap, indentWithTab]),
        editorPlaceholder(props.placeholder),
        EditorView.contentAttributes.of({
          'aria-label': props.label,
          'aria-multiline': 'true',
          spellcheck: 'false',
          'aria-readonly': String(props.readonly),
        }),
        EditorView.theme({
          '&': { height: `${props.height}px`, minHeight: `${props.minHeight}px` },
          '.cm-scroller': { overflow: 'auto' },
          '.cm-content': { fontFamily: 'Consolas, monospace', fontSize: '13px' },
        }),
        ...(props.lineWrapping ? [EditorView.lineWrapping] : []),
        EditorView.updateListener.of((update) => {
          if (update.docChanged && !syncing) emit('update:modelValue', update.state.doc.toString());
        }),
      ],
    }),
  });
});
watch(
  () => props.modelValue,
  (value) => {
    const view = editorView.value;
    if (view && view.state.doc.toString() !== value) {
      syncing = true;
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value ?? '' } });
      syncing = false;
    }
  },
);
watch(
  () => props.mode,
  () => editorView.value?.dispatch({ effects: language.reconfigure(languageExtension()) }),
);
watch(
  () => props.disabled,
  (value) =>
    editorView.value?.dispatch({ effects: editable.reconfigure(EditorView.editable.of(!value)) }),
);
watch(
  () => props.readonly,
  (value) =>
    editorView.value?.dispatch({ effects: readOnly.reconfigure(EditorState.readOnly.of(value)) }),
);
onBeforeUnmount(() => editorView.value?.destroy());
defineExpose({ getView: () => editorView.value });
</script>
<template>
  <div class="code-editor-container"><div ref="editorRef" class="editor-container" /></div>
</template>
