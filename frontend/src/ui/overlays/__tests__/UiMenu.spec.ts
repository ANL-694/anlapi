import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

import UiMenu from '../UiMenu.vue'

const mounted: Array<ReturnType<typeof mount>> = []

afterEach(() => {
  mounted.forEach(wrapper => wrapper.unmount())
  mounted.length = 0
  document.body.innerHTML = ''
})

function mountMenu() {
  const wrapper = mount(UiMenu, {
    attachTo: document.body,
    props: { label: '更多' },
    slots: {
      default: '<button type="button">删除</button>'
    }
  })
  mounted.push(wrapper)
  return wrapper
}

describe('UiMenu', () => {
  it('renders outside an overflow container and aligns with the trigger', async () => {
    const container = document.createElement('div')
    container.style.overflow = 'hidden'
    document.body.appendChild(container)

    const wrapper = mount(UiMenu, {
      attachTo: container,
      props: { label: '更多' },
      slots: { default: '<button type="button">删除</button>' }
    })
    mounted.push(wrapper)

    const trigger = wrapper.get('button')
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue({
      left: 240, right: 280, top: 40, bottom: 76, width: 40, height: 36,
      x: 240, y: 40, toJSON: () => ({})
    })

    await trigger.trigger('click')
    await nextTick()

    const menu = document.body.querySelector<HTMLElement>('.ui-menu-content')
    expect(menu).not.toBeNull()
    expect(container.contains(menu)).toBe(false)
    expect(menu?.style.top).toBe('82px')
    expect(menu?.style.left).toBe('280px')
    expect(menu?.getAttribute('role')).toBe('menu')
    expect(menu?.textContent).toContain('删除')
  })

  it('opens upward when the trigger is near the viewport bottom', async () => {
    const wrapper = mountMenu()
    const trigger = wrapper.get('button')
    const viewportHeight = window.innerHeight
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue({
      left: 240, right: 280, top: viewportHeight - 40, bottom: viewportHeight - 4,
      width: 40, height: 36, x: 240, y: viewportHeight - 40, toJSON: () => ({})
    })
    const originalRect = HTMLElement.prototype.getBoundingClientRect
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function () {
      if (this.classList.contains('ui-menu-content')) {
        return {
          left: 0, right: 192, top: 0, bottom: 120, width: 192, height: 120,
          x: 0, y: 0, toJSON: () => ({})
        }
      }
      return originalRect.call(this)
    })

    try {
      await trigger.trigger('click')
      await nextTick()
      expect(document.body.querySelector<HTMLElement>('.ui-menu-content')?.style.top)
        .toBe(`${viewportHeight - 166}px`)
    } finally {
      vi.restoreAllMocks()
    }
  })

  it('closes on outside click and Escape', async () => {
    const wrapper = mountMenu()
    const trigger = wrapper.get('button')

    await trigger.trigger('click')
    expect(document.body.querySelector('.ui-menu-content')).not.toBeNull()

    document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    await nextTick()
    expect(document.body.querySelector('.ui-menu-content')).toBeNull()

    await trigger.trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(document.body.querySelector('.ui-menu-content')).toBeNull()
  })
})
