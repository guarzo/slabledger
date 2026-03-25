import { DualRangeSlider } from './DualRangeSlider';

export interface GradeRangeSliderProps {
  value: string;
  onChange: (value: string) => void;
  label?: string;
}

const TICKS = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10];

function GradeRangeSlider({ value, onChange, label }: GradeRangeSliderProps) {
  return (
    <DualRangeSlider
      label={label}
      value={value}
      onChange={onChange}
      min={1}
      max={10}
      step={1}
      parseValue={(s) => parseInt(s, 10)}
      formatLabel={(min, max) =>
        min === max ? `PSA ${min}` : `PSA ${min} - PSA ${max}`
      }
      ticks={TICKS}
      minAriaLabel="Minimum PSA grade"
      maxAriaLabel="Maximum PSA grade"
    />
  );
}

GradeRangeSlider.displayName = 'GradeRangeSlider';

export { GradeRangeSlider };
export default GradeRangeSlider;
