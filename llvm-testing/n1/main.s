	.text
	.file	"main.src"
	.globl	ip                      # -- Begin function ip
	.p2align	4, 0x90
	.type	ip,@function
ip:                                     # @ip
	.cfi_startproc
# %bb.0:                                # %input
	movl	$2, %eax
	retq
.Lfunc_end0:
	.size	ip, .Lfunc_end0-ip
	.cfi_endproc
                                        # -- End function
	.globl	op                      # -- Begin function op
	.p2align	4, 0x90
	.type	op,@function
op:                                     # @op
	.cfi_startproc
# %bb.0:                                # %output
	pushq	%rax
	.cfi_def_cfa_offset 16
	movq	%rdi, %rcx
	movl	$.str, %edi
	xorl	%eax, %eax
	movq	%rcx, %rsi
	callq	printf
	popq	%rax
	.cfi_def_cfa_offset 8
	retq
.Lfunc_end1:
	.size	op, .Lfunc_end1-op
	.cfi_endproc
                                        # -- End function
	.globl	add                     # -- Begin function add
	.p2align	4, 0x90
	.type	add,@function
add:                                    # @add
	.cfi_startproc
# %bb.0:                                # %add
	leaq	(%rdi,%rsi), %rax
	retq
.Lfunc_end2:
	.size	add, .Lfunc_end2-add
	.cfi_endproc
                                        # -- End function
	.globl	main                    # -- Begin function main
	.p2align	4, 0x90
	.type	main,@function
main:                                   # @main
	.cfi_startproc
# %bb.0:                                # %main
	pushq	%rbx
	.cfi_def_cfa_offset 16
	.cfi_offset %rbx, -16
	callq	ip
	movq	%rax, %rbx
	callq	ip
	movq	%rbx, %rdi
	movq	%rax, %rsi
	callq	add
	movq	%rax, %rdi
	callq	op
	xorl	%eax, %eax
	popq	%rbx
	.cfi_def_cfa_offset 8
	retq
.Lfunc_end3:
	.size	main, .Lfunc_end3-main
	.cfi_endproc
                                        # -- End function
	.type	.str,@object            # @.str
	.data
	.globl	.str
.str:
	.asciz	"%lld\n"
	.size	.str, 6


	.section	".note.GNU-stack","",@progbits
