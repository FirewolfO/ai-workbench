<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Link, Refresh, Search, Star, StarFilled } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { apiMessage, workbenchApi } from '@/api'
import type { NewsArticle, NewsResult } from '@/types'

const loading = ref(true)
const refreshing = ref(false)
const summarizing = ref(false)
const search = ref('')
const source = ref('')
const favoriteOnly = ref(false)
const result = ref<NewsResult>({ items: [], sources: [], lastError: '' })
let canSummarize: boolean | null = null

const formatDate = (value: string) => new Intl.DateTimeFormat('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
const formatSync = (value?: string) => value ? new Date(value).toLocaleString('zh-CN') : '等待首次同步'

async function load() {
  loading.value = true
  try { result.value = await workbenchApi.news(search.value, source.value, favoriteOnly.value); void ensureChineseSummaries() }
  catch (error) { ElMessage.error(apiMessage(error, '热点加载失败')) }
  finally { loading.value = false }
}
async function ensureChineseSummaries() {
  if (summarizing.value) return
  if (canSummarize === null) {
    try { canSummarize = (await workbenchApi.providers()).some((item) => item.enabled) }
    catch { canSummarize = false }
  }
  if (!canSummarize) return
  const missing = result.value.items.filter((item) => !item.chineseSummary).map((item) => item.id)
  if (!missing.length) return
  summarizing.value = true
  try {
    for (let offset = 0; offset < missing.length; offset += 20) {
      const generated = await workbenchApi.summarizeNews(missing.slice(offset, offset + 20))
      result.value.items.forEach((item) => { if (generated.summaries[item.id]) item.chineseSummary = generated.summaries[item.id] })
    }
  } catch (error) { ElMessage.warning(apiMessage(error, '中文概要生成暂不可用')) }
  finally { summarizing.value = false }
}
async function refresh() {
  refreshing.value = true
  try { const state = await workbenchApi.refreshNews(); ElMessage.success(`已同步 ${state.itemsFetched} 条热点`); await load() }
  catch (error) { ElMessage.error(apiMessage(error, '热点同步失败')) }
  finally { refreshing.value = false }
}
async function toggleFavorite(item: NewsArticle) {
  const next = !item.favorite
  try { await workbenchApi.favoriteNews(item.id, next); item.favorite = next; if (favoriteOnly.value && !next) result.value.items = result.value.items.filter((entry) => entry.id !== item.id) }
  catch (error) { ElMessage.error(apiMessage(error, '收藏更新失败')) }
}
onMounted(load)
</script>

<template>
  <section class="page content-page">
    <header class="page-heading">
      <div><p>DAILY INTELLIGENCE</p><h2>AI 热点</h2><span>OpenAI · Google AI · Hugging Face · arXiv</span></div>
      <el-button :icon="Refresh" :loading="refreshing" @click="refresh">立即同步</el-button>
    </header>
    <div class="content-toolbar">
      <el-input v-model="search" :prefix-icon="Search" clearable placeholder="搜索标题、摘要或作者" @keyup.enter="load" @clear="load" />
      <el-select v-model="source" clearable placeholder="全部来源" @change="load"><el-option v-for="item in result.sources" :key="item.code" :label="item.name" :value="item.code" /></el-select>
      <el-checkbox v-model="favoriteOnly" border @change="load"><el-icon><StarFilled /></el-icon>仅看收藏</el-checkbox>
      <el-button type="primary" @click="load">筛选</el-button>
    </div>
    <div class="sync-line"><span>最近同步：{{ formatSync(result.lastSuccessAt) }}</span><el-tag v-if="summarizing" type="success" effect="plain">正在生成中文概要</el-tag><el-tooltip v-else-if="result.lastError" :content="result.lastError"><el-tag type="warning" effect="plain">部分来源异常</el-tag></el-tooltip></div>
    <div v-loading="loading" class="news-stream">
      <article v-for="item in result.items" :key="item.id">
        <div class="article-meta"><el-tag size="small" effect="plain">{{ item.sourceName }}</el-tag><span>{{ formatDate(item.publishedAt) }}</span><span v-if="item.author">{{ item.author }}</span></div>
        <h3><a :href="item.url" target="_blank" rel="noopener noreferrer">{{ item.title }}</a></h3>
        <p :class="{ 'chinese-summary': item.chineseSummary }"><strong v-if="item.chineseSummary">中文概要</strong>{{ item.chineseSummary || item.summary || '暂无摘要' }}</p>
        <footer>
          <el-button tag="a" :href="item.url" target="_blank" rel="noopener noreferrer" text :icon="Link">查看原文</el-button>
          <el-tooltip :content="item.favorite ? '取消收藏' : '收藏'"><el-button text :icon="item.favorite ? StarFilled : Star" :class="{ favorite: item.favorite }" :aria-label="item.favorite ? '取消收藏' : '收藏'" @click="toggleFavorite(item)" /></el-tooltip>
        </footer>
      </article>
      <el-empty v-if="!loading && !result.items.length" description="没有匹配的热点" />
    </div>
  </section>
</template>

<style scoped>
.content-page{max-width:1080px}.content-toolbar{display:grid;grid-template-columns:minmax(260px,1fr) 180px 130px auto;gap:9px;padding:14px;border:1px solid #dce3e0;background:#fff}.content-toolbar .el-checkbox{margin:0}.sync-line{display:flex;align-items:center;justify-content:space-between;min-height:42px;color:#7b8683;font-size:11px}.news-stream{min-height:260px;border-top:1px solid #dce3e0}.news-stream article{padding:22px 6px;border-bottom:1px solid #dce3e0}.article-meta{display:flex;align-items:center;gap:10px;color:#7b8683;font-size:11px}.news-stream h3{margin:11px 0 8px;font-size:18px;line-height:1.45}.news-stream h3 a{color:#18211f;text-decoration:none}.news-stream h3 a:hover{color:#176b55}.news-stream p{display:-webkit-box;overflow:hidden;max-width:900px;margin:0;color:#65716e;font-size:13px;line-height:1.7;-webkit-box-orient:vertical;-webkit-line-clamp:3}.news-stream p.chinese-summary{color:#34423e}.news-stream p strong{margin-right:8px;color:#176b55;font-size:11px}.news-stream footer{display:flex;align-items:center;justify-content:space-between;margin-top:9px}.news-stream .favorite{color:#c2851d}@media(max-width:760px){.content-toolbar{grid-template-columns:1fr}.news-stream h3{font-size:16px}.article-meta{flex-wrap:wrap}}
</style>
