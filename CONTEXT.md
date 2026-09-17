# userland

A self-hosted platform you switch services on and off in, on one host, in one compose
project. This file is the glossary. It holds no implementation detail.

## Language

Not written yet.

userland consolidates several repos that each carried their own glossary, and that
vocabulary is being re-cut rather than copied over. The central term broke: the word for
*a thing that holds a database on the shared Postgres* used to mean a separate compose
project, and now some of them live inside this one and some outside it. Nothing is
recorded here until that is settled.

Each entry takes the same form the folded repos used — a term, a definition of what it
means in this repo alone, and an explicit *Avoid* list of the words it displaces.
