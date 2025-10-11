// Simple Slider component since it doesn't exist in the UI library
interface SliderProps {
    value: number[]
    onValueChange: (value: number[]) => void
    min: number
    max: number
    step: number
    className?: string
}

export const Slider: React.FC<SliderProps> = ({ value, onValueChange, min, max, step, className = "" }) => {
    return (
        <input
            type="range"
            min={min}
            max={max}
            step={step}
            value={value[0]}
            onChange={(e) => onValueChange([parseFloat(e.target.value)])}
            className={`w-full h-2 bg-gray-200 rounded-lg appearance-none cursor-pointer slider ${className}`}
            style={{
                background: `linear-gradient(to right, hsl(var(--primary)) 0%, hsl(var(--primary)) ${((value[0] - min) / (max - min)) * 100}%, hsl(var(--muted)) ${((value[0] - min) / (max - min)) * 100}%, hsl(var(--muted)) 100%)`
            }}
        />
    )
}
