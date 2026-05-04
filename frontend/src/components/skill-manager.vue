<template>
  <n-space vertical style="margin-bottom: 12px">
    <n-space>
      <n-input
        v-model:value="searchKeyword"
        placeholder="搜索技能名称..."
        style="width: 200px"
        clearable
        @keyup.enter="handleSearch"
      >
        <template #prefix>
          <n-icon :component="SearchOutline" />
        </template>
      </n-input>

      <n-select
        v-model:value="filterCategory"
        :options="categoryOptions"
        placeholder="技能分类"
        style="width: 120px"
        clearable
        filterable
      />

      <n-select
        v-model:value="filterEnable"
        :options="enableOptions"
        placeholder="启用状态"
        style="width: 100px"
        clearable
      />

      <n-button type="primary" @click="handleSearch">
        搜索
      </n-button>

      <n-button type="warning" @click="handleCreate">
        <template #icon>
          <n-icon :component="AddOutline" />
        </template>
        添加技能
      </n-button>
    </n-space>

    <n-data-table
      remote
      :columns="columns"
      :data="tableData"
      :pagination="pagination"
      :loading="loading"
      :row-key="row => row.id"
      @update:page="handlePageChange"
    />
  </n-space>

  <n-modal
    v-model:show="showCreateModal"
    preset="card"
    :title="editingSkill ? '编辑技能' : '添加技能'"
    style="width: 900px; max-height: 90vh"
    :mask-closable="false"
  >
    <div class="modal-body">
    <n-form
      ref="formRef"
      :model="formData"
      :rules="formRules"
      label-placement="top"
      label-align="left"
    >
      <n-grid :cols="4" :x-gap="16">
        <n-form-item-gi label="技能名称" path="name" :span="2">
          <n-input v-model:value="formData.name" placeholder="请输入技能名称" clearable />
        </n-form-item-gi>

        <n-form-item-gi label="分类" path="category">
          <n-select
            v-model:value="formData.category"
            :options="categoryOptions"
            placeholder="选择或输入分类"
            clearable
            filterable
            tag
          />
        </n-form-item-gi>

        <n-form-item-gi label="排序" path="sortOrder">
          <n-input-number v-model:value="formData.sortOrder" :min="0" :max="999" style="width: 100%" />
        </n-form-item-gi>
      </n-grid>

      <n-grid :cols="4" :x-gap="16">
        <n-form-item-gi label="启用" path="enable" :span="1">
          <n-switch v-model:value="formData.enable" />
        </n-form-item-gi>

        <n-form-item-gi label="触发关键词" path="triggerKeywords" :span="3">
          <n-input
            v-model:value="formData.triggerKeywords"
            placeholder="关键词用逗号分隔，例如：技术分析,K线,MACD"
            clearable
          />
        </n-form-item-gi>
      </n-grid>

      <n-form-item label="技能描述" path="description">
        <n-input
          v-model:value="formData.description"
          type="textarea"
          :autosize="{ minRows: 1, maxRows: 3 }"
          placeholder="请输入技能描述"
          show-count
          maxlength="500"
        />
      </n-form-item>

      <n-form-item label="绑定MCP服务" path="mcpServerIds">
        <n-select
          v-model:value="formData.mcpServerIds"
          :options="mcpServerOptions"
          placeholder="选择绑定的MCP服务器"
          multiple
          clearable
        />
      </n-form-item>

      <n-form-item label="系统提示词" path="systemPrompt">
        <MdEditor
          v-model="formData.systemPrompt"
          style="height: 200px"
          :theme="editorTheme"
          :preview="true"
          :toolbarsExclude="['github', 'htmlPreview', 'catalog', 'save']"
          placeholder="当此技能激活时，将追加到系统提示词中，指导 Agent 如何使用此技能"
        />
      </n-form-item>

      <n-form-item label="示例对话" path="examples">
        <MdEditor
          v-model="formData.examples"
          style="height: 160px"
          :theme="editorTheme"
          :preview="true"
          :toolbarsExclude="['github', 'htmlPreview', 'catalog', 'save']"
          placeholder="提供示例对话，帮助 Agent 理解如何使用此技能"
        />
      </n-form-item>
    </n-form>
    </div>

    <template #footer>
      <n-space justify="end">
        <n-button @click="showCreateModal = false">取消</n-button>
        <n-button type="primary" :loading="submitting" @click="handleSubmit">
          {{ editingSkill ? '保存' : '创建' }}
        </n-button>
      </n-space>
    </template>
  </n-modal>

</template>

<script setup>
import { ref, reactive, h, onMounted } from 'vue'
import {
  NButton, NSpace, NInput, NDataTable, NModal, NForm, NFormItem,
  NFormItemGi, NGrid, NTag, NSwitch, NIcon, NSelect, NInputNumber, NPopconfirm, NScrollbar, useMessage
} from 'naive-ui'
import { SearchOutline, AddOutline, TrashOutline, CreateOutline, FlashOutline } from '@vicons/ionicons5'
import { MdEditor } from 'md-editor-v3'
import 'md-editor-v3/lib/style.css'
import { CreateSkill, UpdateSkill, DeleteSkill, GetSkillList, EnableSkill, GetSkillByID, GetAllSkills } from '../../wailsjs/go/main/App.js'
import { GetMCPServerList, GetConfig } from '../../wailsjs/go/main/App.js'

const message = useMessage()
const loading = ref(false)
const submitting = ref(false)
const searchKeyword = ref('')
const filterCategory = ref(null)
const filterEnable = ref(null)
const showCreateModal = ref(false)
const editingSkill = ref(false)
const formRef = ref(null)
const tableData = ref([])
const mcpServerOptions = ref([])
const editorTheme = ref('light')


const currentPage = ref(1)
const pageSize = ref(10)
const total = ref(0)

const pagination = reactive({
  page: 1,
  pageCount: 1,
  pageSize: 10,
  itemCount: 0,
  showSizePicker: true,
  pageSizes: [10, 20, 50],
  prefix: ({ itemCount }) => `共 ${itemCount} 条`,
  onChange: (page) => {
    handlePageChange(page)
  },
  onUpdatePageSize: (size) => {
    pageSize.value = size
    pagination.pageSize = size
    currentPage.value = 1
    pagination.page = 1
    loadData()
  }
})

const formData = reactive({
  id: null,
  name: '',
  description: '',
  category: null,
  systemPrompt: '',
  examples: '',
  triggerKeywords: '',
  mcpServerIds: [],
  enable: true,
  sortOrder: 0
})

const formRules = {
  name: { required: true, message: '请输入技能名称', trigger: ['input', 'blur'] }
}

const categoryOptions = [
  { label: '股票分析', value: '股票分析' },
  { label: '技术分析', value: '技术分析' },
  { label: '基本面分析', value: '基本面分析' },
  { label: '量化策略', value: '量化策略' },
  { label: '风险管理', value: '风险管理' },
  { label: '资讯研究', value: '资讯研究' },
  { label: '通用', value: '通用' }
]

const enableOptions = [
  { label: '已启用', value: true },
  { label: '已禁用', value: false }
]

const columns = [
  {
    title: 'ID',
    key: 'id',
    width: 50
  },
  {
    title: '技能名称',
    key: 'name',
    width: 120,
    ellipsis: { tooltip: true }
  },
  {
    title: '分类',
    key: 'category',
    width: 90,
    render(row) {
      if (!row.category) return h(NTag, { type: 'default' }, { default: () => '未分类' })
      return h(NTag, { type: 'info' }, { default: () => row.category })
    }
  },
  {
    title: '描述',
    key: 'description',
    width: 200,
    ellipsis: { tooltip: { style: { maxWidth: '400px', wordBreak: 'break-all' } } }
  },
  {
    title: '绑定MCP',
    key: 'mcpServerIds',
    width: 100,
    render(row) {
      if (!row.mcpServerIds) return h(NTag, { type: 'default' }, { default: () => '无' })
      const ids = row.mcpServerIds.split(',').filter(s => s.trim())
      return h(NTag, { type: 'info' }, { default: () => `${ids.length} 个` })
    }
  },
  {
    title: '排序',
    key: 'sortOrder',
    width: 60
  },
  {
    title: '启用',
    key: 'enable',
    width: 70,
    render(row) {
      return h(NSwitch, {
        value: row.enable,
        onUpdateValue: (val) => handleEnable(row, val)
      })
    }
  },
  {
    title: '操作',
    key: 'actions',
    width: 140,
    render(row) {
      return h(NSpace, { size: 'small' }, {
        default: () => [
          h(NButton, {
            size: 'small', type: 'info', quaternary: true,
            onClick: () => handleEdit(row)
          }, {
            icon: () => h(NIcon, null, { default: () => h(CreateOutline) }),
            default: () => '编辑'
          }),
          h(NPopconfirm, {
            onPositiveClick: () => handleDelete(row)
          }, {
            trigger: () => h(NButton, {
              size: 'small', type: 'error', quaternary: true
            }, {
              icon: () => h(NIcon, null, { default: () => h(TrashOutline) }),
              default: () => '删除'
            }),
            default: () => '确定删除此技能？'
          })
        ]
      })
    }
  }
]

const loadData = async () => {
  loading.value = true
  try {
    const result = await GetSkillList({
      page: currentPage.value,
      pageSize: pageSize.value,
      name: searchKeyword.value,
      category: filterCategory.value,
      enable: filterEnable.value
    })
    if (result) {
      tableData.value = result.data || []
      total.value = result.total || 0
      pagination.itemCount = total.value
      pagination.pageCount = Math.ceil(total.value / pageSize.value) || 1
    }
  } catch (error) {
    message.error('加载数据失败: ' + error)
  } finally {
    loading.value = false
  }
}

const loadMCPServers = async () => {
  try {
    const result = await GetMCPServerList({
      page: 1,
      pageSize: 100,
      name: '',
      status: '',
      enable: true
    })
    if (result && result.data) {
      mcpServerOptions.value = result.data.map(s => ({
        label: s.name,
        value: String(s.id)
      }))
    }
  } catch (error) {
    console.error('加载MCP服务器列表失败:', error)
  }
}

const handlePageChange = (page) => {
  currentPage.value = page
  pagination.page = page
  loadData()
}

const handleSearch = () => {
  currentPage.value = 1
  pagination.page = 1
  loadData()
}

const handleCreate = () => {
  editingSkill.value = false
  resetForm()
  showCreateModal.value = true
}

const handleEdit = async (row) => {
  editingSkill.value = true
  try {
    const skill = await GetSkillByID(row.id)
    if (skill) {
      resetForm()
      formData.id = skill.id
      formData.name = skill.name
      formData.description = skill.description
      formData.category = skill.category
      formData.systemPrompt = skill.systemPrompt
      formData.examples = skill.examples
      formData.triggerKeywords = skill.triggerKeywords
      formData.mcpServerIds = skill.mcpServerIds ? skill.mcpServerIds.split(',').filter(s => s.trim()) : []
      formData.enable = skill.enable
      formData.sortOrder = skill.sortOrder
      showCreateModal.value = true
    }
  } catch (error) {
    message.error('获取技能信息失败: ' + error)
  }
}

const handleDelete = async (row) => {
  try {
    const result = await DeleteSkill(row.id)
    if (result.includes('成功')) {
      message.success(result)
      loadData()
    } else {
      message.error(result)
    }
  } catch (error) {
    message.error('删除失败: ' + error)
  }
}

const handleEnable = async (row, enable) => {
  try {
    const result = await EnableSkill(row.id, enable)
    if (result.includes('成功') || result.includes('启用') || result.includes('禁用')) {
      message.success(result)
      loadData()
    } else {
      message.error(result)
    }
  } catch (error) {
    message.error('操作失败: ' + error)
  }
}

const handleSubmit = async () => {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }

  submitting.value = true
  try {
    const skillData = {
      name: formData.name,
      description: formData.description,
      category: formData.category || '',
      systemPrompt: formData.systemPrompt,
      examples: formData.examples,
      triggerKeywords: formData.triggerKeywords,
      mcpServerIds: formData.mcpServerIds.join(','),
      enable: formData.enable,
      sortOrder: formData.sortOrder
    }

    let result
    if (editingSkill.value) {
      skillData.id = formData.id
      result = await UpdateSkill(skillData)
    } else {
      result = await CreateSkill(skillData)
    }

    if (result.includes('成功')) {
      message.success(result)
      showCreateModal.value = false
      loadData()
    } else {
      message.error(result)
    }
  } catch (error) {
    message.error('操作失败: ' + error)
  } finally {
    submitting.value = false
  }
}

const resetForm = () => {
  Object.assign(formData, {
    id: null,
    name: '',
    description: '',
    category: null,
    systemPrompt: '',
    examples: '',
    triggerKeywords: '',
    mcpServerIds: [],
    enable: true,
    sortOrder: 0
  })
  if (formRef.value) {
    formRef.value.restoreValidation()
  }
}

onMounted(() => {
  loadData()
  loadMCPServers()
  GetConfig().then(result => {
    if (result.darkTheme) {
      editorTheme.value = 'dark'
    }
  })
})
</script>

<style scoped>
.modal-body {
  max-height: calc(85vh - 160px);
  overflow-y: auto;
  padding-right: 8px;
}
:deep(.md-editor) {
  text-align: left;
}
</style>
