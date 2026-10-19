import { defineComponent, h, type PropType } from 'vue'
import { icons, type IconName } from './icons'

/** One icon from the set, decorative unless a label is given. */
export default defineComponent({
  name: 'AppIconGlyph',
  props: {
    name: { type: String as PropType<IconName>, required: true },
    label: { type: String, default: '' },
  },
  setup(props) {
    return () =>
      h('svg', {
        class: 'i',
        viewBox: '0 0 24 24',
        'aria-hidden': props.label ? undefined : 'true',
        'aria-label': props.label || undefined,
        role: props.label ? 'img' : undefined,
        innerHTML: icons[props.name],
      })
  },
})
