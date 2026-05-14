import { useEffect, useRef } from 'react'

export function MeshBackground() {
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return

    const ctx = canvas.getContext('2d')
    if (!ctx) return

    let animationFrameId: number
    let width = window.innerWidth
    let height = window.innerHeight

    const particles: Particle[] = []
    const particleCount = Math.min(Math.floor((width * height) / 10000), 120)
    const connectionDistance = 180
    const mouse = { x: -1000, y: -1000 }

    class Particle {
      x: number
      y: number
      vx: number
      vy: number
      size: number
      pulse: number
      pulseSpeed: number

      constructor() {
        this.x = Math.random() * width
        this.y = Math.random() * height
        this.vx = (Math.random() - 0.5) * 0.4
        this.vy = (Math.random() - 0.5) * 0.4
        this.size = Math.random() * 2 + 1
        this.pulse = Math.random() * Math.PI
        this.pulseSpeed = 0.02 + Math.random() * 0.03
      }

      update() {
        this.x += this.vx
        this.y += this.vy
        this.pulse += this.pulseSpeed

        if (this.x < 0 || this.x > width) this.vx *= -1
        if (this.y < 0 || this.y > height) this.vy *= -1

        // Mouse interaction: Nodes are attracted to mouse
        const dx = mouse.x - this.x
        const dy = mouse.y - this.y
        const dist = Math.sqrt(dx * dx + dy * dy)
        if (dist < 200) {
          const force = (200 - dist) / 200
          this.vx += dx * force * 0.001
          this.vy += dy * force * 0.001
        }
        
        // Friction to prevent infinite acceleration
        this.vx *= 0.99
        this.vy *= 0.99
      }

      draw() {
        if (!ctx) return
        ctx.beginPath()
        const opacity = 0.4 + Math.sin(this.pulse) * 0.3
        ctx.arc(this.x, this.y, this.size * (1 + Math.sin(this.pulse) * 0.2), 0, Math.PI * 2)
        ctx.fillStyle = `rgba(139, 92, 246, ${opacity})` // Brand accent purple
        ctx.fill()
        
        // Highlights on nodes
        if (Math.sin(this.pulse) > 0.8) {
          ctx.beginPath()
          ctx.arc(this.x, this.y, this.size * 2, 0, Math.PI * 2)
          ctx.fillStyle = `rgba(167, 139, 250, 0.2)`
          ctx.fill()
        }
      }
    }

    const init = () => {
      particles.length = 0
      for (let i = 0; i < particleCount; i++) {
        particles.push(new Particle())
      }
    }

    const resize = () => {
      width = window.innerWidth
      height = window.innerHeight
      canvas.width = width
      canvas.height = height
      init()
    }

    const animate = () => {
      ctx.clearRect(0, 0, width, height)

      for (let i = 0; i < particles.length; i++) {
        const p1 = particles[i]
        p1.update()
        p1.draw()

        for (let j = i + 1; j < particles.length; j++) {
          const p2 = particles[j]
          const dx = p1.x - p2.x
          const dy = p1.y - p2.y
          const dist = Math.sqrt(dx * dx + dy * dy)

          if (dist < connectionDistance) {
            ctx.beginPath()
            ctx.moveTo(p1.x, p1.y)
            ctx.lineTo(p2.x, p2.y)
            const opacity = (1 - dist / connectionDistance) * 0.3
            
            // Mouse interaction with connections
            const mdx = mouse.x - (p1.x + p2.x) / 2
            const mdy = mouse.y - (p1.y + p2.y) / 2
            const mdist = Math.sqrt(mdx * mdx + mdy * mdy)
            const highlight = mdist < 150 ? (150 - mdist) / 150 : 0
            
            ctx.strokeStyle = `rgba(139, 92, 246, ${opacity + highlight * 0.4})`
            ctx.lineWidth = 1 + highlight
            ctx.stroke()
          }
        }
      }

      animationFrameId = requestAnimationFrame(animate)
    }

    window.addEventListener('resize', resize)
    window.addEventListener('mousemove', (e) => {
      mouse.x = e.clientX
      mouse.y = e.clientY
    })

    resize()
    animate()

    return () => {
      window.removeEventListener('resize', resize)
      cancelAnimationFrame(animationFrameId)
    }
  }, [])

  return (
    <canvas
      ref={canvasRef}
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 0,
        pointerEvents: 'none',
        opacity: 0.6,
      }}
    />
  )
}
