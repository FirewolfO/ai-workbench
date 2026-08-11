<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Delete, Link, Plus, Refresh, Search, Star, StarFilled } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiMessage, workbenchApi } from '@/api'
import type { PeopleResult, SocialPost, TrackedPerson } from '@/types'

const loading = ref(true)
const refreshing = ref(false)
const dialogOpen = ref(false)
const saving = ref(false)
const people = ref<PeopleResult>({ people: [], xConfigured: false, lastError: '' })
const posts = ref<SocialPost[]>([])
const selectedID = ref('')
const search = ref('')
const favoriteOnly = ref(false)
const form = reactive({ handle: '', displayName: '' })
const selected = computed(() => people.value.people.find((item) => item.id === selectedID.value))
const formatDate = (value: string) => new Date(value).toLocaleString('zh-CN')
const formatSync = (value?: string) => value ? new Date(value).toLocaleString('zh-CN') : '等待首次同步'

async function loadPeople() {
  people.value = await workbenchApi.people()
  if (!selectedID.value || !people.value.people.some((item) => item.id === selectedID.value)) selectedID.value = ''
}
async function loadPosts() { posts.value = await workbenchApi.socialPosts(selectedID.value, search.value, favoriteOnly.value) }
async function load() {
  loading.value = true
  try { await Promise.all([loadPeople(), loadPosts()]) }
  catch (error) { ElMessage.error(apiMessage(error, '人物动态加载失败')) }
  finally { loading.value = false }
}
async function choose(person?: TrackedPerson) { selectedID.value = person?.id || ''; await loadPosts() }
async function refresh() {
  refreshing.value = true
  try { const state = await workbenchApi.refreshPeople(); ElMessage.success(`已同步 ${state.itemsFetched} 条动态`); await load() }
  catch (error) { ElMessage.error(apiMessage(error, '人物动态同步失败')) }
  finally { refreshing.value = false }
}
function openAdd() { Object.assign(form, { handle: '', displayName: '' }); dialogOpen.value = true }
async function add() {
  if (!form.handle.trim()) { ElMessage.warning('请输入 X 用户名'); return }
  saving.value = true
  try { await workbenchApi.addPerson(form.handle, form.displayName); dialogOpen.value = false; await loadPeople(); ElMessage.success('已添加关注人') }
  catch (error) { ElMessage.error(apiMessage(error, '添加失败')) }
  finally { saving.value = false }
}
async function remove(person: TrackedPerson) {
  try { await ElMessageBox.confirm(`停止关注 @${person.handle}？`, '移除关注人', { type: 'warning' }); await workbenchApi.deletePerson(person.id); await load() }
  catch (error) { if (error !== 'cancel' && error !== 'close') ElMessage.error(apiMessage(error, '移除失败')) }
}
async function toggleFavorite(post: SocialPost) {
  const next = !post.favorite
  try { await workbenchApi.favoritePost(post.id, next); post.favorite = next; if (favoriteOnly.value && !next) posts.value = posts.value.filter((item) => item.id !== post.id) }
  catch (error) { ElMessage.error(apiMessage(error, '收藏更新失败')) }
}
onMounted(load)
</script>

<template>
  <section class="page people-page">
    <header class="page-heading">
      <div><p>PEOPLE WATCH</p><h2>大佬动态</h2><span>以 Tibor Blaho 的 Codex 产品动态为主</span></div>
      <el-button type="primary" :icon="Refresh" :loading="refreshing" :disabled="!people.xConfigured" @click="refresh">立即同步</el-button>
    </header>
    <el-alert v-if="!people.xConfigured" title="X 数据源尚未启用" description="请在 AI Workbench 后端配置 AI_WORKBENCH_X_BEARER_TOKEN。关注列表和收藏仍会正常保存。" type="warning" :closable="false" show-icon />
    <div v-loading="loading" class="people-workspace">
      <aside>
        <header><div><strong>关注的人</strong><small>{{ people.people.length }} 人</small></div><el-tooltip content="添加关注人"><el-button circle size="small" :icon="Plus" aria-label="添加关注人" @click="openAdd" /></el-tooltip></header>
        <button :class="{ active: !selectedID }" type="button" @click="choose()"><span class="person-avatar">ALL</span><span><strong>全部动态</strong><small>按发布时间汇总</small></span></button>
        <div v-for="person in people.people" :key="person.id" class="person-row" :class="{ active: selectedID === person.id }" role="button" tabindex="0" @click="choose(person)" @keydown.enter="choose(person)">
          <span class="person-avatar">{{ person.displayName.slice(0, 1).toUpperCase() }}</span><span><strong>{{ person.displayName }}</strong><small>@{{ person.handle }}</small></span>
          <el-tooltip v-if="person.handle !== 'btibor91'" content="移除关注"><el-button text :icon="Delete" aria-label="移除关注" @click.stop="remove(person)" /></el-tooltip>
        </div>
      </aside>
      <main>
        <div class="people-toolbar"><div><strong>{{ selected?.displayName || '全部动态' }}</strong><small>最近同步：{{ formatSync(people.lastSuccessAt) }}</small></div><el-input v-model="search" :prefix-icon="Search" clearable placeholder="搜索动态" @keyup.enter="loadPosts" @clear="loadPosts" /><el-checkbox v-model="favoriteOnly" border @change="loadPosts"><el-icon><StarFilled /></el-icon>仅看收藏</el-checkbox></div>
        <div class="post-stream">
          <article v-for="post in posts" :key="post.id">
            <span class="person-avatar">{{ post.displayName.slice(0, 1).toUpperCase() }}</span>
            <div><header><strong>{{ post.displayName }}</strong><span>@{{ post.handle }} · {{ formatDate(post.publishedAt) }}</span><el-tooltip :content="post.favorite ? '取消收藏' : '收藏'"><el-button text :icon="post.favorite ? StarFilled : Star" :class="{ favorite: post.favorite }" :aria-label="post.favorite ? '取消收藏' : '收藏'" @click="toggleFavorite(post)" /></el-tooltip></header><p>{{ post.content }}</p><footer><span>{{ post.replyCount }} 回复 · {{ post.repostCount }} 转发 · {{ post.likeCount }} 喜欢</span><el-button tag="a" :href="post.url" target="_blank" rel="noopener noreferrer" text :icon="Link">在 X 查看</el-button></footer></div>
          </article>
          <el-empty v-if="!posts.length" :description="people.xConfigured ? '暂无匹配动态' : '配置 X API 后即可同步动态'" />
        </div>
      </main>
    </div>
    <el-dialog v-model="dialogOpen" title="添加 X 关注人" width="min(460px,94vw)"><el-form label-position="top"><el-form-item label="X 用户名"><el-input v-model="form.handle" maxlength="16" placeholder="例如 btibor91"><template #prepend>@</template></el-input></el-form-item><el-form-item label="备注名称"><el-input v-model="form.displayName" maxlength="160" placeholder="留空后首次同步自动获取" /></el-form-item></el-form><template #footer><el-button @click="dialogOpen=false">取消</el-button><el-button type="primary" :loading="saving" @click="add">添加</el-button></template></el-dialog>
  </section>
</template>

<style scoped>
.people-page{max-width:1180px}.people-page>.el-alert{margin-bottom:14px}.people-workspace{display:grid;grid-template-columns:245px minmax(0,1fr);min-height:600px;border:1px solid #dce3e0;background:#fff}.people-workspace>aside{padding:12px 8px;border-right:1px solid #dce3e0;background:#f8faf9}.people-workspace>aside>header{display:flex;align-items:center;justify-content:space-between;min-height:48px;padding:0 8px 8px}.people-workspace>aside>header div>*{display:block}.people-workspace>aside>header strong{font-size:13px}.people-workspace>aside>header small{margin-top:3px;color:#7b8683;font-size:10px}.people-workspace>aside>button{display:grid;grid-template-columns:34px minmax(0,1fr) 28px;align-items:center;gap:9px;width:100%;min-height:56px;padding:7px;border:0;border-radius:5px;color:#26312e;text-align:left;background:transparent;cursor:pointer}.people-workspace>aside>button:hover,.people-workspace>aside>button.active{background:#e8f0ee}.people-workspace>aside>button>span:nth-child(2){min-width:0}.people-workspace>aside>button strong,.people-workspace>aside>button small{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.people-workspace>aside>button strong{font-size:12px}.people-workspace>aside>button small{margin-top:3px;color:#74807d;font-size:10px}.person-avatar{display:grid;place-items:center;width:32px;height:32px;border-radius:6px;color:#fff;background:#176b55;font-size:10px;font-weight:750}.people-workspace>main{min-width:0}.people-toolbar{display:grid;grid-template-columns:minmax(160px,1fr) minmax(180px,280px) 130px;align-items:center;gap:10px;min-height:68px;padding:10px 16px;border-bottom:1px solid #dce3e0}.people-toolbar>div strong,.people-toolbar>div small{display:block}.people-toolbar>div strong{font-size:13px}.people-toolbar>div small{margin-top:4px;color:#7b8683;font-size:10px}.people-toolbar .el-checkbox{margin:0}.post-stream{padding:0 20px}.post-stream article{display:grid;grid-template-columns:34px minmax(0,1fr);gap:11px;padding:20px 0;border-bottom:1px solid #e5e9e7}.post-stream article>div{min-width:0}.post-stream article header{display:flex;align-items:center;min-height:28px;gap:7px}.post-stream article header strong{font-size:13px}.post-stream article header span{flex:1;color:#7b8683;font-size:11px}.post-stream article p{margin:7px 0 9px;font-size:14px;line-height:1.7;white-space:pre-wrap;overflow-wrap:anywhere}.post-stream article footer{display:flex;align-items:center;justify-content:space-between;color:#7b8683;font-size:10px}.post-stream .favorite{color:#c2851d}@media(max-width:760px){.people-workspace{grid-template-columns:76px minmax(0,1fr)}.people-workspace>aside{padding:8px 5px}.people-workspace>aside>header div,.people-workspace>aside>button>span:nth-child(2),.people-workspace>aside>button>.el-button{display:none}.people-workspace>aside>header{justify-content:center}.people-workspace>aside>button{grid-template-columns:1fr;justify-items:center}.people-toolbar{grid-template-columns:1fr;padding:12px}.post-stream{padding:0 10px}.post-stream article header{flex-wrap:wrap}.post-stream article header span{order:3;flex-basis:100%}}
.person-row{display:grid;grid-template-columns:34px minmax(0,1fr) 28px;align-items:center;gap:9px;width:100%;min-height:56px;padding:7px;border:0;border-radius:5px;color:#26312e;text-align:left;background:transparent;cursor:pointer}.person-row:hover,.person-row.active{background:#e8f0ee}.person-row>span:nth-child(2){min-width:0}.person-row strong,.person-row small{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.person-row strong{font-size:12px}.person-row small{margin-top:3px;color:#74807d;font-size:10px}@media(max-width:760px){.person-row{grid-template-columns:1fr;justify-items:center}.person-row>span:nth-child(2),.person-row>.el-button{display:none}}
</style>
