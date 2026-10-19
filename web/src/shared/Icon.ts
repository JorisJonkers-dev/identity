import { defineComponent, h, type PropType } from 'vue'
import { icons, type IconName } from './icons'

/** One icon from the set; decorative, so the text beside it names it. */
export default defineComponent({
  name: 'AppIconGlyph',
  props: {
    name: { type: String as PropType<IconName>, required: true },
  },
  setup(props) {
    return () =>
      h('svg', {
        class: 'i',
        viewBox: '0 0 24 24',
        'aria-hidden': 'true',
        innerHTML: icons[props.name],
      })
  },
})
