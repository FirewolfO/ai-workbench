<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Delete, EditPen, Plus, User } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiMessage, workbenchApi } from '@/api'
import type { InternalUser } from '@/types'

const loading = ref(true)
const saving = ref(false)
const dialogOpen = ref(false)
const passwordOpen = ref(false)
const users = ref<InternalUser[]>([])
const form = reactive({ username: '', displayName: '', password: '' })
const passwordForm = reactive({ username: '', password: '' })

async function load() {
  loading.value = true
  try { users.value = await workbenchApi.users() }
  catch (error) { ElMessage.error(apiMessage(error, '用户加载失败')) }
  finally { loading.value = false }
}
function openCreate() { Object.assign(form, { username: '', displayName: '', password: '' }); dialogOpen.value = true }
async function create() {
  if (!form.username.trim()) { ElMessage.warning('请输入用户名'); return }
  saving.value = true
  try {
    const result = await workbenchApi.createUser({ ...form, password: form.password || undefined })
    dialogOpen.value = false
    await ElMessageBox.alert(`初始密码：${result.initialPassword}`, `用户 ${result.user.username} 已创建`, { confirmButtonText: '知道了' })
    await load()
  } catch (error) { ElMessage.error(apiMessage(error, '创建失败')) }
  finally { saving.value = false }
}
async function toggle(item: InternalUser) {
  try { await workbenchApi.updateUser(item.username, { enabled: !item.enabled }); await load() }
  catch (error) { ElMessage.error(apiMessage(error, '状态更新失败')) }
}
function openPassword(item: InternalUser) { Object.assign(passwordForm, { username: item.username, password: '' }); passwordOpen.value = true }
async function resetPassword() {
  if (passwordForm.password.length < 8) { ElMessage.warning('密码至少需要 8 个字符'); return }
  saving.value = true
  try { await workbenchApi.updateUser(passwordForm.username, { password: passwordForm.password }); passwordOpen.value = false; ElMessage.success('密码已更新，原会话已失效') }
  catch (error) { ElMessage.error(apiMessage(error, '密码更新失败')) }
  finally { saving.value = false }
}
async function remove(item: InternalUser) {
  try { await ElMessageBox.confirm(`确定删除用户“${item.username}”吗？`, '删除用户', { type: 'warning' }); await workbenchApi.deleteUser(item.username); await load() }
  catch (error) { if (error !== 'cancel' && error !== 'close') ElMessage.error(apiMessage(error, '删除失败')) }
}
onMounted(load)
</script>

<template>
  <section class="page users-page">
    <div class="page-heading"><div><p>INTERNAL ACCOUNTS</p><h2>用户管理</h2><span>管理 AI Workbench 内部登录账号</span></div><el-button type="primary" :icon="Plus" @click="openCreate">添加用户</el-button></div>
    <div v-loading="loading" class="user-table-wrap">
      <el-table :data="users" row-key="username">
        <el-table-column label="用户" min-width="220"><template #default="{ row }"><div class="user-cell"><span><el-icon><User /></el-icon></span><div><strong>{{ row.displayName }}</strong><small>{{ row.username }}</small></div></div></template></el-table-column>
        <el-table-column label="角色" width="120"><template #default="{ row }"><el-tag :type="row.role === 'admin' ? 'success' : 'info'" effect="plain">{{ row.role === 'admin' ? '管理员' : '普通用户' }}</el-tag></template></el-table-column>
        <el-table-column label="状态" width="120"><template #default="{ row }"><el-switch :model-value="row.enabled" :disabled="row.role === 'admin'" @change="toggle(row)" /></template></el-table-column>
        <el-table-column label="创建时间" min-width="170"><template #default="{ row }">{{ new Date(row.createdAt).toLocaleString('zh-CN') }}</template></el-table-column>
        <el-table-column label="操作" width="150" align="right"><template #default="{ row }"><el-tooltip content="重设密码"><el-button text :icon="EditPen" aria-label="重设密码" @click="openPassword(row)" /></el-tooltip><el-tooltip content="删除用户"><el-button text type="danger" :icon="Delete" aria-label="删除用户" :disabled="row.role === 'admin'" @click="remove(row)" /></el-tooltip></template></el-table-column>
      </el-table>
    </div>
    <el-dialog v-model="dialogOpen" title="添加内部用户" width="min(480px, 94vw)"><el-form label-position="top"><el-form-item label="用户名"><el-input v-model="form.username" maxlength="40" autocomplete="off" /></el-form-item><el-form-item label="显示名称"><el-input v-model="form.displayName" maxlength="120" /></el-form-item><el-form-item label="初始密码"><el-input v-model="form.password" type="password" show-password autocomplete="new-password" placeholder="留空则使用 用户名@123" /></el-form-item></el-form><template #footer><el-button @click="dialogOpen = false">取消</el-button><el-button type="primary" :loading="saving" @click="create">创建</el-button></template></el-dialog>
    <el-dialog v-model="passwordOpen" title="重设密码" width="min(440px, 94vw)"><el-form label-position="top"><el-form-item :label="passwordForm.username"><el-input v-model="passwordForm.password" type="password" show-password autocomplete="new-password" placeholder="至少 8 个字符" @keyup.enter="resetPassword" /></el-form-item></el-form><template #footer><el-button @click="passwordOpen = false">取消</el-button><el-button type="primary" :loading="saving" @click="resetPassword">更新密码</el-button></template></el-dialog>
  </section>
</template>
