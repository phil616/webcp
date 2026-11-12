<script setup lang="ts">
import { useRoute } from 'vue-router';
import { router } from '@/router';
import { navigate } from '@/navigation';
import PagesView from '@/views/PagesView.vue';
import { computed, ref } from 'vue';
import { FolderOpened } from '@element-plus/icons-vue';
import { useManager, request } from '@/api';
import type { Site } from '@/types';
import LayoutContent from '@/vendor/1panel/components/layout-content/index.vue';
import TableSearch from '@/vendor/1panel/components/table/TableSearch.vue';
import TableRefresh from '@/vendor/1panel/components/table/TableRefresh.vue';
import TableOperationActions from '@/vendor/1panel/components/table/TableOperationActions.vue';
import type { FuTableOperationButton } from '@/vendor/1panel/components/table/shared';
import Status from '@/vendor/1panel/components/status/index.vue';
import ComplexTable from '@/components/panel/ComplexTable.vue';
import SiteEditor from '@/components/SiteEditor.vue';
import WebsiteSettings from '@/components/website/WebsiteSettings.vue';
import WebsiteFiles from '@/components/panel/WebsiteFiles.vue';
import DeleteDialog from '@/components/panel/DeleteDialog.vue';
const { state, busy, act, refresh } = useManager();
const search = ref(''),
  kind = ref(''),
  creating = ref(false),
  filesFor = ref<Site>(),
  selected = ref<Site[]>([]),
  defaultPagesOpen = ref(false);
const confirmRef = ref<InstanceType<typeof DeleteDialog>>();
const route = useRoute();
const configID = computed(() => String(route.query.site ?? ''));
const configTab = computed(() => String(route.query.tab ?? 'basic'));
const editing = computed(() => state.value?.sites.find((site) => site.id === configID.value));
const sites = computed(
  () =>
    state.value?.sites
      .filter(
        (site) =>
          (!kind.value || site.kind === kind.value) &&
          `${site.name} ${site.domains.join(' ')}`
            .toLowerCase()
            .includes((search.value ?? '').toLowerCase()),
      )
      .sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt)) ?? [],
);
function configure(site: Site, tab = 'basic') {
  void router.push({ name: 'Website', query: { site: site.id, tab } });
}
async function remove(sites: Site[]) {
  if (!sites.length) return;
  if (
    await confirmRef.value?.ask({
      title: '删除网站',
      message: '删除网站配置后，应用时将停止提供这些网站。磁盘文件保留。',
      names: sites.map((site) => site.name),
    })
  )
    await act(
      async () => {
        for (const site of sites) {
          await request(`/sites/${site.id}`, 'DELETE');
          await refresh();
        }
        selected.value = [];
      },
      '删除成功',
      { deploy: true },
    );
}
async function toggle(site: Site) {
  if (
    await confirmRef.value?.ask({
      title: site.enabled ? '停止网站' : '启动网站',
      message: site.enabled ? '停止后，此网站将无法访问。' : '确认启用此网站？',
      names: site.domains,
    })
  )
    await act(() => request('/sites', 'POST', { ...site, enabled: !site.enabled }), '操作成功', {
      deploy: true,
    });
}
const operations: FuTableOperationButton<Site>[] = [
  { label: '配置', click: configure },
  { label: '备份', click: () => navigate('backups') },
  { label: '网站目录', click: (site) => (filesFor.value = site) },
  {
    label: '删除',
    click: (site) => {
      void remove([site]);
    },
    type: 'danger',
  },
];
</script>
<template>
  <div class="route-view">
    <WebsiteSettings
      v-if="editing"
      :key="`${editing.id}:${configTab}`"
      :site="editing"
      :initial-tab="configTab"
    /><LayoutContent v-else
      ><template #leftToolBar
        ><el-button type="primary" @click="creating = true">创建网站</el-button
        ><el-button type="primary" plain @click="defaultPagesOpen = true">高级设置</el-button
        ><el-button :disabled="!selected.length || busy" @click="remove(selected)"
          >删除</el-button
        ></template
      ><template #rightToolBar
        ><el-select v-model="kind" class="filter-select" aria-label="网站类型筛选"
          ><template #prefix>类型</template><el-option value="" label="全部" /><el-option
            value="static"
            label="静态网站" /><el-option value="proxy" label="反向代理" /></el-select
        ><TableSearch v-model:search-name="search" placeholder="搜索网站名称或域名" /><TableRefresh
          aria-label="刷新网站"
          :disabled="busy"
          @search="refresh" /></template
      ><template #main
        ><ComplexTable :data="sites" selection @selection-change="(rows) => (selected = rows)"
          ><el-table-column label="名称" card-type="name" min-width="240"
            ><template #default="{ row }"
              ><div class="table-name">
                <el-button type="primary" link @click="configure(row as Site)">{{
                  row.name
                }}</el-button
                ><span class="table-subtitle">{{ row.domains.join(' · ') }}</span>
              </div></template
            ></el-table-column
          ><el-table-column label="类型" width="130" card-type="content"
            ><template #default="{ row }">{{
              row.kind === 'static' ? '静态网站' : '反向代理'
            }}</template></el-table-column
          ><el-table-column label="网站目录" width="100" align="center" card-type="content"
            ><template #default="{ row }"
              ><el-button
                type="primary"
                link
                :icon="FolderOpened"
                aria-label="网站文件"
                @click="filesFor = row as Site" /></template></el-table-column
          ><el-table-column label="状态" card-type="status" width="130"
            ><template #default="{ row }"
              ><Status
                :status="row.enabled ? 'enable' : 'disable'"
                operate
                :disabled="busy"
                @click="toggle(row as Site)" /></template></el-table-column
          ><el-table-column label="HTTPS" width="130" card-type="content"
            ><template #default="{ row }"
              ><el-button type="primary" link @click="configure(row as Site, 'tls')">{{
                row.tlsId ? '已启用' : '未启用'
              }}</el-button></template
            ></el-table-column
          ><el-table-column label="Lua 扩展" width="100" card-type="content"
            ><template #default="{ row }">{{
              Object.keys(row.luaBindings ?? {}).length || '—'
            }}</template></el-table-column
          ><el-table-column label="操作" card-type="button" width="200" fixed="right"
            ><template #default="{ row }"
              ><TableOperationActions
                :buttons="operations"
                :row="row as Site"
                :ellipsis="2"
                trigger="click" /></template></el-table-column
          ><template #footerLeft>网站文件即时生效，配置保存后校验并应用。</template></ComplexTable
        ></template
      ></LayoutContent
    ><SiteEditor v-if="creating" :site="null" @close="creating = false" /><WebsiteFiles
      v-if="filesFor"
      :site="filesFor"
      @close="filesFor = undefined"
    /><PagesView v-if="defaultPagesOpen" drawer @close="defaultPagesOpen = false" /><DeleteDialog
      ref="confirmRef"
    />
  </div>
</template>
