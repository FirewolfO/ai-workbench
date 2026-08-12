<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Compass, Link, Search, Star, TrendCharts } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { apiMessage, workbenchApi } from '@/api'
import type { FrontierCategory, FrontierResult } from '@/types'

const loading = ref(true)
const filters = reactive<{ search: string; category: FrontierCategory; language: string; period: string; sort: string }>({ search: '', category: 'project', language: '', period: '90d', sort: 'recommended' })
const result = ref<FrontierResult>({ items: [], total: 0, generatedAt: '', githubTokenSet: false, rateLimit: { limit: 0, remaining: 0 }, stale: false })
const categories = [{ label: 'AI 项目', value: 'project' }, { label: 'Skills', value: 'skill' }, { label: '插件', value: 'plugin' }]
const languages = ['Python', 'TypeScript', 'JavaScript', 'Go', 'Rust', 'Java', 'Shell']
const categoryName = computed(() => categories.find((item) => item.value === filters.category)?.label || '项目')
const compact = (value: number) => new Intl.NumberFormat('zh-CN', { notation: 'compact', maximumFractionDigits: 1 }).format(value)
const activeDate = (value: string) => new Intl.RelativeTimeFormat('zh-CN', { numeric: 'auto' }).format(-Math.max(0, Math.round((Date.now() - new Date(value).getTime()) / 86400000)), 'day')
const syncedAt = computed(() => result.value.lastSuccessAt ? new Date(result.value.lastSuccessAt).toLocaleString('zh-CN') : '等待首次同步')

async function load() {
  loading.value = true
  try { result.value = await workbenchApi.frontier(filters) }
  catch (error) { ElMessage.error(apiMessage(error, '前沿项目加载失败')) }
  finally { loading.value = false }
}
function changeCategory(value: string | number | boolean) { filters.category = String(value) as FrontierCategory; void load() }
onMounted(load)
</script>

<template>
  <div class="frontier-page">
    <header class="frontier-heading">
      <div><span><el-icon><Compass /></el-icon> GITHUB DISCOVERY</span><h2>前沿项目</h2><p>从活跃度、社区规模与成熟度中发现值得关注的开源项目</p></div>
      <el-segmented :model-value="filters.category" :options="categories" @change="changeCategory" />
    </header>

    <section class="frontier-toolbar">
      <el-input v-model="filters.search" :prefix-icon="Search" clearable placeholder="搜索方向或关键词" @keyup.enter="load" @clear="load" />
      <el-select v-model="filters.language" clearable placeholder="全部语言" @change="load"><el-option v-for="item in languages" :key="item" :label="item" :value="item" /></el-select>
      <el-select v-model="filters.period" aria-label="活跃时间" @change="load"><el-option label="近 30 天活跃" value="30d" /><el-option label="近 90 天活跃" value="90d" /><el-option label="近半年活跃" value="180d" /><el-option label="近一年活跃" value="1y" /><el-option label="不限时间" value="all" /></el-select>
      <el-select v-model="filters.sort" aria-label="排序方式" @change="load"><el-option label="综合推荐" value="recommended" /><el-option label="Star 最多" value="stars" /><el-option label="最近更新" value="updated" /><el-option label="最新创建" value="newest" /></el-select>
      <el-button type="primary" :icon="Search" @click="load">发现</el-button>
    </section>

    <div class="frontier-status"><span>每天 11:00 自动更新 · 最近同步：{{ syncedAt }}</span><span v-if="result.stale">GitHub 暂时不可用，当前展示缓存结果</span><span v-else>找到 {{ result.total.toLocaleString() }} 个相关{{ categoryName }}<template v-if="result.rateLimit.limit"> · GitHub 额度 {{ result.rateLimit.remaining }}/{{ result.rateLimit.limit }}</template></span></div>

    <section v-loading="loading" class="frontier-grid">
      <article v-for="item in result.items" :key="item.id">
        <header><img :src="item.ownerAvatar" :alt="item.owner" /><div><small>{{ item.owner }}</small><h3><a :href="item.url" target="_blank" rel="noopener noreferrer">{{ item.name }}</a></h3></div><el-tooltip content="综合推荐分"><strong class="quality-score">{{ item.score }}</strong></el-tooltip></header>
        <p>{{ item.description || '该项目暂未提供简介。' }}</p>
        <div class="frontier-signals"><el-tag v-for="signal in item.signals.slice(0, 3)" :key="signal" size="small" effect="plain">{{ signal }}</el-tag></div>
        <dl><div><dt><el-icon><Star /></el-icon> Stars</dt><dd>{{ compact(item.stars) }}</dd></div><div><dt><el-icon><TrendCharts /></el-icon> Forks</dt><dd>{{ compact(item.forks) }}</dd></div><div><dt>活跃</dt><dd>{{ activeDate(item.pushedAt) }}</dd></div></dl>
        <div class="topic-line"><span v-if="item.language" class="language-dot">{{ item.language }}</span><span v-if="item.license">{{ item.license }}</span><span v-for="topic in item.topics.slice(0, 2)" :key="topic">#{{ topic }}</span></div>
        <footer><span>{{ item.fullName }}</span><el-button tag="a" :href="item.url" target="_blank" rel="noopener noreferrer" text :icon="Link">查看仓库</el-button></footer>
      </article>
      <el-empty v-if="!loading && !result.items.length" :description="`没有匹配的${categoryName}`" />
    </section>
  </div>
</template>

<style scoped>
.frontier-page{width:min(1180px,calc(100% - 48px));margin:0 auto;padding:32px 0 44px}.frontier-heading{display:flex;align-items:flex-end;justify-content:space-between;gap:24px;margin-bottom:24px}.frontier-heading>div>span{display:flex;align-items:center;gap:6px;margin-bottom:8px;color:#176b55;font-size:10px;font-weight:800}.frontier-heading h2{margin:0;font-size:26px;letter-spacing:0}.frontier-heading p{margin:7px 0 0;color:#6c7774;font-size:13px}.frontier-toolbar{display:grid;grid-template-columns:minmax(230px,1fr) 150px 165px 140px auto;gap:8px;padding:14px;border:1px solid #dfe7e4;background:#fff}.frontier-status{display:flex;justify-content:space-between;min-height:44px;padding:14px 2px 10px;color:#6c7774;font-size:11px}.frontier-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px;min-height:260px}.frontier-grid article{display:flex;flex-direction:column;min-width:0;min-height:300px;padding:18px;border:1px solid #dfe7e4;border-radius:6px;background:#fff}.frontier-grid article>header{display:grid;grid-template-columns:38px minmax(0,1fr) 42px;align-items:center;gap:10px}.frontier-grid img{width:38px;height:38px;border-radius:6px}.frontier-grid header small,.frontier-grid header h3{display:block;overflow:hidden;margin:0;text-overflow:ellipsis;white-space:nowrap}.frontier-grid header small{color:#6c7774;font-size:10px}.frontier-grid header h3{margin-top:3px;font-size:16px}.frontier-grid a{color:#1e2b28;text-decoration:none}.frontier-grid a:hover{color:#176b55}.quality-score{display:grid;place-items:center;width:40px;height:34px;border:1px solid #b9d8cf;border-radius:5px;color:#176b55;background:#edf7f3;font-size:14px}.frontier-grid article>p{display:-webkit-box;overflow:hidden;min-height:42px;margin:16px 0 12px;color:#56635f;font-size:12px;line-height:1.7;-webkit-box-orient:vertical;-webkit-line-clamp:2}.frontier-signals{display:flex;flex-wrap:wrap;gap:5px;min-height:24px}.frontier-grid dl{margin:15px 0 11px;padding:10px 0;border-block:1px solid #e7edeb}.frontier-grid dl div{display:grid;grid-template-columns:1fr auto;align-items:center;margin:5px 0;font-size:11px}.frontier-grid dt{display:flex;align-items:center;gap:5px;color:#6c7774}.frontier-grid dd{margin:0;font-weight:700}.topic-line{display:flex;flex-wrap:wrap;gap:8px;min-height:32px;color:#6c7774;font-size:10px}.topic-line .language-dot{color:#176b55;font-weight:700}.frontier-grid footer{display:flex;align-items:center;justify-content:space-between;gap:8px;margin-top:auto}.frontier-grid footer>span{overflow:hidden;color:#78837f;font-size:10px;text-overflow:ellipsis;white-space:nowrap}.frontier-grid :deep(.el-empty){grid-column:1/-1}@media(max-width:900px){.frontier-toolbar{grid-template-columns:1fr 1fr}.frontier-toolbar>.el-input{grid-column:1/-1}.frontier-grid{grid-template-columns:1fr}}@media(max-width:600px){.frontier-page{width:calc(100% - 24px);padding-top:22px}.frontier-heading{align-items:flex-start;flex-direction:column}.frontier-heading :deep(.el-segmented){width:100%}.frontier-toolbar{grid-template-columns:1fr}.frontier-toolbar>.el-input{grid-column:auto}.frontier-status{align-items:flex-start;flex-direction:column;gap:5px}.frontier-grid article{padding:14px}}
</style>
