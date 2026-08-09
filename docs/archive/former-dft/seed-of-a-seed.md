My goal is to create a full ai-native development environment and workflow.

I have bought a new GMTek Evo V2 mini PC with 96Gb of ram that I intend to use as my LLM server. My initial idea is to give assign 75Gb to vram. 

My development process will follow the spec-kit methodology for the development process.
## Philosophy
While the bootstrapping of the project will be done in interactive mode, we should move to recursive development dogfooding our creation, using the framework to develop its own enhancements.
## The Project

### Phase 1
What I want is to first create a Hermes-based process that would:
- develop an [[#Initial application]]  using speckit (using the **specify workflow run** command)
- using an eval harness that we will also develop as part of this process, judge the quality of the output
- this process must be reusable. I will evolve the speckit orchestrator and verification process dogfooding the dev environment and I also want to run the comparison tests across different types of applications
- the whole process must be autonomous, dark factory style
- the goal is not the initial application itself, but the dev process
- all of this we will do with frontier models (gpt family, 5.4 and below depending on task)

### Phase 2
We will setup the development environment (the Evo V2) remotely:
- research and identify the best candidates as the LLM backend for development given the hardware described above.
- download the most promising candidates and create the configuration to run them using ollama
- Install ollama


### Initial application
Custom Flashcard & Quiz App
A flashcard web app where users can create decks, flip cards, and take randomized quizzes. Implement a spaced-repetition algorithm that shows missed cards more frequently.
### How to Evaluate the Output
We should design a harness to evaluate the quatily of the artifacts created by the dev process. Implement two types of evaluation:
#### AI based
Judge its 
- **Modularity:** Are the CSS, HTML, and JavaScript separated cleanly?
- **Documentation:** Did it include clear instructions on how to run or install the application?
#### BDD
Create a bdd harness setup to test the application from the user's perspective.
Test the CRUD operations, egde cases, etc.