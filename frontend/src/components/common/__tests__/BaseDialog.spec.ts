import { afterEach, describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import BaseDialog from '@/components/common/BaseDialog.vue'

const wrappers: VueWrapper[] = []

function mountDialog(showCloseButton?: boolean) {
  const wrapper = mount(BaseDialog, {
    attachTo: document.body,
    props: {
      show: true,
      title: '测试弹窗',
      ...(showCloseButton === undefined ? {} : { showCloseButton }),
    },
    global: {
      stubs: {
        Icon: true,
      },
    },
  })
  wrappers.push(wrapper)
  return wrapper
}

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) {
    wrapper.unmount()
  }
  document.body.innerHTML = ''
})

describe('BaseDialog', () => {
  it('默认显示标题栏关闭按钮', () => {
    mountDialog()
    expect(document.body.querySelector('.modal-close-button')).not.toBeNull()
  })

  it('强制确认场景可以隐藏标题栏关闭按钮', () => {
    mountDialog(false)
    expect(document.body.querySelector('.modal-close-button')).toBeNull()
  })
})
